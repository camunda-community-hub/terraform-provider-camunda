package provider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// errNotFound is returned (wrapped) by consoleClient whenever the requested
// object does not exist. Check it with errors.Is.
var errNotFound = errors.New("not found")

// consoleClientConfig holds everything needed to talk to the Console API.
type consoleClientConfig struct {
	APIURL       string
	TokenURL     string
	Audience     string
	ClientID     string
	ClientSecret string
	Debug        bool
}

// consoleClient is the provider's only door to the Camunda Console API.
//
// It owns authentication (an OAuth2 client-credentials token that is refreshed
// on expiry), error formatting, not-found semantics and waiting for clusters to
// become healthy. Resources and data sources only map between Terraform state
// and the values returned here.
//
// Every method returns an error that already carries the API response body,
// wrapping errNotFound when the object is missing. Deciding what a missing
// object means is left to the lifecycle module.
type consoleClient struct {
	api *console.DefaultAPIService

	// Organization members have no single-member endpoint, so every lookup
	// needs the whole list. It is cached to keep N member resources from
	// fetching the same N-sized list N times.
	members memberCache

	// Cluster health polling; overridden in tests.
	clusterWaitTimeout  time.Duration
	clusterWaitDelay    time.Duration
	clusterPollInterval time.Duration
}

// newConsoleClient builds a client and fetches a first token, so that invalid
// credentials surface while configuring the provider rather than on first use.
func newConsoleClient(ctx context.Context, cfg consoleClientConfig) (*consoleClient, error) {
	apiURL, err := url.Parse(cfg.APIURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse API URL: %w", err)
	}

	credentials := clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		EndpointParams: url.Values{
			"audience": []string{cfg.Audience},
		},
	}

	// The token source keeps this context for every later refresh, so it must
	// outlive the provider's Configure request.
	tokenSource := credentials.TokenSource(context.WithoutCancel(ctx))
	if _, err := tokenSource.Token(); err != nil {
		return nil, fmt.Errorf("unable to get token: %w", err)
	}

	apiCfg := console.NewConfiguration()
	apiCfg.Scheme = apiURL.Scheme
	apiCfg.Host = apiURL.Host
	apiCfg.Debug = cfg.Debug
	// The retries sit outside the oauth2 transport, so every attempt gets a
	// current token even if the waits outlast the previous one.
	apiCfg.HTTPClient = &http.Client{
		Transport: newRateLimitTransport(&oauth2.Transport{
			Source: tokenSource,
			Base:   http.DefaultTransport,
		}),
	}

	return &consoleClient{
		api:                 console.NewAPIClient(apiCfg).DefaultAPI,
		clusterWaitTimeout:  30 * time.Minute,
		clusterWaitDelay:    10 * time.Second,
		clusterPollInterval: 5 * time.Second,
	}, nil
}

// apiError turns a generated-client error into one that carries the response
// body, wrapping errNotFound on HTTP 404.
func apiError(err error, response *http.Response) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	var openAPIErr *console.GenericOpenAPIError
	if errors.As(err, &openAPIErr) && len(openAPIErr.Body()) > 0 {
		msg = fmt.Sprintf("%s: %s", msg, openAPIErr.Body())
	}

	// The response is nil when the request never reached the server.
	if response != nil && response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", errNotFound, msg)
	}

	return errors.New(msg)
}

// Clusters

func (c *consoleClient) CreateCluster(ctx context.Context, cl cluster) (string, error) {
	req := console.CreateClusterRequest{
		Name:         cl.Name,
		PlanTypeId:   cl.PlanTypeID,
		ChannelId:    cl.ChannelID,
		GenerationId: cl.GenerationID,
		RegionId:     cl.RegionID,
		AutoUpdate:   &cl.AutoUpdate,
	}
	if cl.Description != "" {
		req.Description = &cl.Description
	}
	if cl.BackupRegionID != "" {
		req.BackupRegionId = &cl.BackupRegionID
	}
	if cl.Encryption != "" {
		encryption, err := console.NewClusterEncryptionKeyFromValue(cl.Encryption)
		if err != nil {
			return "", fmt.Errorf("unable to read encryption: %w", err)
		}
		req.Encryption = encryption
	}
	if cl.StageLabel != "" {
		stage, err := console.NewCamundaClusterStageFromValue(cl.StageLabel)
		if err != nil {
			return "", fmt.Errorf("unable to read stage label: %w", err)
		}
		req.StageLabel = stage
	}
	if cl.HardwarePackages != 0 {
		packages := float64(cl.HardwarePackages)
		req.HardwarePackages = &packages
	}

	created, response, err := c.api.CreateCluster(ctx).CreateClusterRequest(req).Execute()
	if err != nil {
		return "", apiError(err, response)
	}
	return created.GetClusterId(), nil
}

// GetCluster returns the cluster, or errNotFound.
func (c *consoleClient) GetCluster(ctx context.Context, clusterID string) (*cluster, error) {
	found, err := c.getAPICluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	return newCluster(found), nil
}

// ListClusters returns all clusters of the organization.
func (c *consoleClient) ListClusters(ctx context.Context) ([]cluster, error) {
	listed, response, err := c.api.GetClusters(ctx).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	clusters := make([]cluster, 0, len(listed))
	for i := range listed {
		clusters = append(clusters, *newCluster(&listed[i]))
	}
	return clusters, nil
}

// newCluster converts the API's cluster to the provider's.
func newCluster(found *console.Cluster) *cluster {
	cl := &cluster{
		ID:           found.Uuid,
		Name:         found.Name,
		Description:  found.GetDescription(),
		ChannelID:    found.Channel.Uuid,
		RegionID:     found.Region.Uuid,
		PlanTypeID:   found.PlanType.Uuid,
		GenerationID: found.Generation.Uuid,
		AutoUpdate:   found.AutoUpdate,
		OwnerID:      found.OwnerId,
		Created:      found.Created,
		Labels:       found.Labels,
		Status:       string(found.Status.Ready),
		Links: clusterLinks{
			Zeebe:      found.Links.GetZeebe(),
			Operate:    found.Links.GetOperate(),
			Tasklist:   found.Links.GetTasklist(),
			Optimize:   found.Links.GetOptimize(),
			Connectors: found.Links.GetConnectors(),
			Identity:   found.Links.GetIdentity(),
			Console:    found.Links.GetConsole(),
			OAuth:      found.Links.GetOauth(),
		},
	}
	if found.BackupRegion != nil {
		cl.BackupRegionID = found.BackupRegion.Uuid
	}
	if found.Encryption != nil {
		cl.Encryption = string(found.Encryption.Type)
		cl.EncryptionStatus = string(found.Encryption.GetStatus())
		cl.EncryptionPrimaryKeyARN = found.Encryption.GetPrimaryKeyArn()
		cl.EncryptionSecondaryKeyARN = found.Encryption.GetSecondaryKeyArn()
	}
	return cl
}

func (c *consoleClient) getAPICluster(ctx context.Context, clusterID string) (*console.Cluster, error) {
	found, response, err := c.api.GetCluster(ctx, clusterID).Execute()
	return found, apiError(err, response)
}

// UpdateCluster applies the update in place.
func (c *consoleClient) UpdateCluster(ctx context.Context, clusterID string, update clusterUpdate) error {
	body := console.UpdateClusterBody{Name: &update.Name, Description: &update.Description}
	if update.StageLabel != "" {
		stage, err := console.NewCamundaClusterStageFromValue(update.StageLabel)
		if err != nil {
			return fmt.Errorf("unable to read stage label: %w", err)
		}
		body.StageLabel = stage
	}
	if update.HardwarePackages != 0 {
		packages := float64(update.HardwarePackages)
		body.NumberOfAllocatedHwPackages = &packages
	}

	response, err := c.api.UpdateCluster(withReplayableWrite(ctx), clusterID).UpdateClusterBody(body).Execute()
	return apiError(err, response)
}

func (c *consoleClient) DeleteCluster(ctx context.Context, clusterID string) error {
	response, err := c.api.DeleteCluster(ctx, clusterID).Execute()
	return apiError(err, response)
}

// WaitClusterHealthy blocks until the cluster reports healthy twice in a row.
func (c *consoleClient) WaitClusterHealthy(ctx context.Context, clusterID string) error {
	return c.awaitStatus(ctx, statusWait{
		pending: []string{
			string(console.CLUSTERCOMPONENTSTATUS_CREATING),
			string(console.CLUSTERCOMPONENTSTATUS_UPDATING),
		},
		target:      []string{string(console.CLUSTERCOMPONENTSTATUS_HEALTHY)},
		consecutive: 2,
		refresh: func() (string, error) {
			cluster, err := c.getAPICluster(ctx, clusterID)
			if err != nil {
				return "", err
			}

			tflog.Info(ctx, "Camunda cluster status", map[string]interface{}{
				"clusterID":     cluster.Uuid,
				"clusterStatus": cluster.Status.Ready,
			})

			return string(cluster.Status.Ready), nil
		},
	})
}

// statusWait describes how to wait for an asynchronous Console operation, such
// as a backup or an activation, to settle.
type statusWait struct {
	// pending are the statuses that mean keep waiting. Any status that is
	// neither pending nor a target, such as "Failed", ends the wait with an
	// error naming it.
	pending []string
	target  []string
	// consecutive is how often in a row a target status must be seen; 0 means 1.
	consecutive int
	// refresh returns the current status. An error ends the wait, so it must
	// wrap errNotFound when the object disappears.
	refresh func() (string, error)
}

// awaitStatus polls w.refresh until a target status shows up, using the
// client's cluster wait timeout, delay and poll interval.
func (c *consoleClient) awaitStatus(ctx context.Context, w statusWait) error {
	wait := &retry.StateChangeConf{
		Pending:                   w.pending,
		Target:                    w.target,
		ContinuousTargetOccurence: max(w.consecutive, 1),
		Refresh: func() (interface{}, string, error) {
			status, err := w.refresh()
			if err != nil {
				return nil, "", err
			}
			return status, status, nil
		},
		Timeout:    c.clusterWaitTimeout,
		Delay:      c.clusterWaitDelay,
		MinTimeout: c.clusterPollInterval,
	}

	_, err := wait.WaitForStateContext(ctx)
	return err
}

// GetIPAllowlist returns the cluster's IP allowlist.
func (c *consoleClient) GetIPAllowlist(ctx context.Context, clusterID string) ([]allowlistEntry, error) {
	found, err := c.getAPICluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	listed := found.Ipallowlist
	if listed == nil {
		// Older API responses only carry the deprecated field.
		listed = found.Ipwhitelist
	}

	entries := []allowlistEntry{}
	for _, entry := range listed {
		entries = append(entries, allowlistEntry{IP: entry.Ip, Description: entry.Description})
	}
	return entries, nil
}

// SetIPAllowlist replaces the cluster's IP allowlist; an empty list removes it.
func (c *consoleClient) SetIPAllowlist(ctx context.Context, clusterID string, entries []allowlistEntry) error {
	body := []console.ClusterIpallowlistInner{}
	for _, entry := range entries {
		body = append(body, *console.NewClusterIpallowlistInner(entry.Description, entry.IP))
	}

	response, err := c.api.UpdateIpAllowlist(ctx, clusterID).
		IpAllowListBody(console.IpAllowListBody{Ipallowlist: body}).
		Execute()
	return apiError(err, response)
}

// Cluster clients

// CreateClusterClient creates a client and returns its ID and secret. The
// secret is only ever returned here.
func (c *consoleClient) CreateClusterClient(ctx context.Context, clusterID, name string, scopes []string) (*createdClusterClient, error) {
	created, response, err := c.api.CreateClient(ctx, clusterID).
		CreateClusterClientBody(console.CreateClusterClientBody{
			ClientName:  name,
			Permissions: scopes,
		}).
		Execute()
	if err != nil {
		return nil, apiError(err, response)
	}
	return &createdClusterClient{ClientID: created.ClientId, Secret: created.ClientSecret}, nil
}

// GetClusterClient returns the client with its connection details and scopes,
// or errNotFound.
func (c *consoleClient) GetClusterClient(ctx context.Context, clusterID, clientID string) (*clusterClient, error) {
	details, response, err := c.api.GetClient(ctx, clusterID, clientID).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	// Only the client list carries the scopes.
	clients, response, err := c.api.GetClients(ctx, clusterID).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	for _, listed := range clients {
		if listed.ClientId == clientID {
			return &clusterClient{
				ClientID:               details.ZEEBE_CLIENT_ID,
				Name:                   details.Name,
				Scopes:                 listed.Permissions,
				ZeebeAddress:           details.ZEEBE_ADDRESS,
				AuthorizationServerURL: details.ZEEBE_AUTHORIZATION_SERVER_URL,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: client %q in cluster %s", errNotFound, clientID, clusterID)
}

// ListClusterClients returns the cluster's clients with their scopes and
// connection details, or errNotFound when the cluster does not exist.
func (c *consoleClient) ListClusterClients(ctx context.Context, clusterID string) ([]clusterClient, error) {
	listed, response, err := c.api.GetClients(ctx, clusterID).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	// The Console doesn't order the list, and Terraform keeps it in order.
	slices.SortFunc(listed, func(a, b console.ClusterClient) int { return strings.Compare(a.ClientId, b.ClientId) })

	clients := make([]clusterClient, 0, len(listed))
	for _, entry := range listed {
		details, response, err := c.api.GetClient(ctx, clusterID, entry.ClientId).Execute()
		if err != nil {
			return nil, apiError(err, response)
		}
		clients = append(clients, clusterClient{
			ClientID:               details.ZEEBE_CLIENT_ID,
			Name:                   details.Name,
			Scopes:                 entry.Permissions,
			ZeebeAddress:           details.ZEEBE_ADDRESS,
			AuthorizationServerURL: details.ZEEBE_AUTHORIZATION_SERVER_URL,
		})
	}
	return clients, nil
}

func (c *consoleClient) DeleteClusterClient(ctx context.Context, clusterID, clientID string) error {
	response, err := c.api.DeleteClient(ctx, clusterID, clientID).Execute()
	return apiError(err, response)
}

// Connector secrets

func (c *consoleClient) CreateSecret(ctx context.Context, clusterID, name, value string) error {
	response, err := c.api.CreateSecret(ctx, clusterID).
		CreateSecretBody(console.CreateSecretBody{SecretName: name, SecretValue: value}).
		Execute()
	return apiError(err, response)
}

// GetSecret returns the secret's value, or errNotFound when either the cluster
// or the secret does not exist.
func (c *consoleClient) GetSecret(ctx context.Context, clusterID, name string) (string, error) {
	secrets, response, err := c.api.GetSecrets(ctx, clusterID).Execute()
	if err != nil {
		return "", apiError(err, response)
	}

	value, ok := secrets[name]
	if !ok {
		return "", fmt.Errorf("%w: connector secret %q in cluster %s", errNotFound, name, clusterID)
	}
	return value, nil
}

// ListSecretNames returns the names of the cluster's connector secrets, or
// errNotFound when the cluster does not exist. The values are deliberately
// not returned.
func (c *consoleClient) ListSecretNames(ctx context.Context, clusterID string) ([]string, error) {
	secrets, response, err := c.api.GetSecrets(ctx, clusterID).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}
	return slices.Sorted(maps.Keys(secrets)), nil
}

func (c *consoleClient) DeleteSecret(ctx context.Context, clusterID, name string) error {
	response, err := c.api.DeleteSecret(ctx, clusterID, name).Execute()
	return apiError(err, response)
}

// Organization members

// errOwnerUnchangeable is returned (wrapped) when asked to change or remove
// the organization owner, which the API doesn't allow.
var errOwnerUnchangeable = errors.New("the organization owner's roles can't be changed and the owner can't be removed through the API")

// assignableMemberRoles are the organization roles the API can assign. Other
// roles a member may hold, such as owner, are never returned or sent.
var assignableMemberRoles = []string{
	string(console.ORGANIZATIONROLEADMIN_ADMIN),
	string(console.ORGANIZATIONROLEOPERATIONSENGINEER_OPERATIONSENGINEER),
	string(console.ORGANIZATIONROLETASKUSER_TASKUSER),
	string(console.ORGANIZATIONROLEANALYST_ANALYST),
	string(console.ORGANIZATIONROLEDEVELOPER_DEVELOPER),
	string(console.ORGANIZATIONROLEVISITOR_VISITOR),
}

// memberCacheTTL bounds how long a fetched member list is reused. It only has
// to span one Terraform operation; changes made outside of it show up on the
// next one.
const memberCacheTTL = 30 * time.Second

// memberCache holds the organization's member list. Writes through the client
// update it in place, so applying many members does not refetch the list.
type memberCache struct {
	mu        sync.Mutex
	members   []console.Member
	fetchedAt time.Time
	// invited are the members added by a write, whose cached entry only has
	// an email and roles. Reads that need the complete member refetch the
	// list while any are left.
	invited map[string]bool

	// Overridden in tests.
	now func() time.Time
}

func (m *memberCache) time() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// GetMember returns the member with the given email, or errNotFound. Roles
// holds only assignable roles; Owner reports the owner role.
//
// The Console API only offers the full member list, which is cached for
// memberCacheTTL. Concurrent callers wait for a single fetch.
func (c *consoleClient) GetMember(ctx context.Context, email string) (*member, error) {
	return c.getMember(ctx, email, false)
}

// GetMemberDetails is GetMember for callers that report everything about the
// member, such as the member's name and whether the invitation is pending. It
// refetches the list when the member was just invited, because the cache then
// only knows their email and roles.
func (c *consoleClient) GetMemberDetails(ctx context.Context, email string) (*member, error) {
	return c.getMember(ctx, email, true)
}

func (c *consoleClient) getMember(ctx context.Context, email string, complete bool) (*member, error) {
	c.members.mu.Lock()
	defer c.members.mu.Unlock()

	members, err := c.cachedMembers(ctx, complete)
	if err != nil {
		return nil, err
	}

	for _, listed := range members {
		if listed.Email == email {
			found := newMember(listed)
			return &found, nil
		}
	}
	return nil, fmt.Errorf("%w: organization member %q", errNotFound, email)
}

// ListMembers returns all organization members, from the same cache as
// GetMember, refetched when it holds members that were just invited.
func (c *consoleClient) ListMembers(ctx context.Context) ([]member, error) {
	c.members.mu.Lock()
	defer c.members.mu.Unlock()

	listed, err := c.cachedMembers(ctx, true)
	if err != nil {
		return nil, err
	}

	members := make([]member, 0, len(listed))
	for _, m := range listed {
		members = append(members, newMember(m))
	}
	return members, nil
}

// cachedMembers returns the member list, fetching it when the cache is empty
// or older than memberCacheTTL, or, if complete, when it holds invited members
// that are not fully known. The caller holds c.members.mu.
func (c *consoleClient) cachedMembers(ctx context.Context, complete bool) ([]console.Member, error) {
	stale := c.members.members == nil || c.members.time().Sub(c.members.fetchedAt) >= memberCacheTTL
	if stale || (complete && len(c.members.invited) > 0) {
		members, response, err := c.api.GetMembers(ctx).Execute()
		if err != nil {
			return nil, apiError(err, response)
		}
		if members == nil {
			members = []console.Member{}
		}
		c.members.members = members
		c.members.fetchedAt = c.members.time()
		c.members.invited = nil
	}
	return c.members.members, nil
}

// newMember converts a listed member, keeping only assignable roles and
// reporting the owner role as Owner.
func newMember(listed console.Member) member {
	found := member{
		Email:         listed.Email,
		Name:          listed.Name,
		InvitePending: listed.InvitePending,
		Roles:         []string{},
		AllRoles:      []string{},
	}
	for _, role := range listed.Roles {
		found.AllRoles = append(found.AllRoles, string(role))
		if role == console.ORGANIZATIONROLE_OWNER {
			found.Owner = true
		}
		if slices.Contains(assignableMemberRoles, string(role)) {
			found.Roles = append(found.Roles, string(role))
		}
	}
	return found
}

// SetMemberRoles invites the member if needed and replaces their roles, or
// returns errOwnerUnchangeable for the owner.
func (c *consoleClient) SetMemberRoles(ctx context.Context, email string, roles []string) error {
	existing, err := c.GetMember(ctx, email)
	switch {
	case errors.Is(err, errNotFound):
		// Not a member yet; UpdateMembers invites them.
	case err != nil:
		return err
	case existing.Owner:
		return fmt.Errorf("%w: %s", errOwnerUnchangeable, email)
	}

	orgRoles := make([]console.AssignableOrganizationRoleType, 0, len(roles))
	for _, name := range roles {
		role, err := console.NewAssignableOrganizationRoleTypeFromValue(name)
		if err != nil {
			return fmt.Errorf("unable to read role: %w", err)
		}
		orgRoles = append(orgRoles, *role)
	}

	response, err := c.api.UpdateMembers(withReplayableWrite(ctx), email).
		PostMemberBody(console.PostMemberBody{OrgRoles: orgRoles}).
		Execute()
	if err != nil {
		return apiError(err, response)
	}

	c.members.setRoles(email, orgRoles)
	return nil
}

// DeleteMember removes the member, or returns errOwnerUnchangeable for the
// owner and errNotFound when they aren't a member.
func (c *consoleClient) DeleteMember(ctx context.Context, email string) error {
	existing, err := c.GetMember(ctx, email)
	if err != nil {
		return err
	}
	if existing.Owner {
		return fmt.Errorf("%w: %s", errOwnerUnchangeable, email)
	}

	response, err := c.api.DeleteMember(ctx, email).Execute()
	err = apiError(err, response)
	// A member that is already gone must not linger in the cache either.
	if err == nil || errors.Is(err, errNotFound) {
		c.members.remove(email)
	}
	return err
}

// setRoles records the roles just sent to the API. A member that is not cached
// yet was invited, so it is added.
func (m *memberCache) setRoles(email string, assigned []console.AssignableOrganizationRoleType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.members == nil {
		return
	}

	roles := make([]console.OrganizationRole, 0, len(assigned))
	for _, role := range assigned {
		roles = append(roles, console.OrganizationRole(role))
	}

	for i := range m.members {
		if m.members[i].Email == email {
			m.members[i].Roles = roles
			return
		}
	}
	m.members = append(m.members, console.Member{Email: email, Roles: roles})
	if m.invited == nil {
		m.invited = map[string]bool{}
	}
	m.invited[email] = true
}

func (m *memberCache) remove(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.members = slices.DeleteFunc(m.members, func(member console.Member) bool {
		return member.Email == email
	})
	delete(m.invited, email)
}

// Parameters

// GetParameters returns the channels, plan types and regions available to the
// organization.
func (c *consoleClient) GetParameters(ctx context.Context) (*parameters, error) {
	params, response, err := c.api.GetParameters(ctx).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	result := &parameters{}
	for _, ch := range params.Channels {
		listed := channel{
			ID:                ch.Uuid,
			Name:              ch.Name,
			DefaultGeneration: namedID{ID: ch.DefaultGeneration.Uuid, Name: ch.DefaultGeneration.Name},
		}
		for _, generation := range ch.AllowedGenerations {
			listed.AllowedGenerations = append(listed.AllowedGenerations, namedID{ID: generation.Uuid, Name: generation.Name})
		}
		result.Channels = append(result.Channels, listed)
	}
	for _, region := range params.Regions {
		result.Regions = append(result.Regions, namedID{ID: region.Uuid, Name: region.Name})
	}
	for _, planType := range params.ClusterPlanTypes {
		result.PlanTypes = append(result.PlanTypes, namedID{ID: planType.Uuid, Name: planType.Name})
	}
	return result, nil
}

// Meta

// GetIPRanges returns the IP ranges Camunda's services connect from, per
// service.
func (c *consoleClient) GetIPRanges(ctx context.Context) (*ipRanges, error) {
	meta, response, err := c.api.GetMeta(ctx).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}
	return &ipRanges{WebModeler: meta.WebModeler}, nil
}

// Activity

// GetAuditLog returns the organization's activity events.
func (c *consoleClient) GetAuditLog(ctx context.Context) ([]auditEvent, error) {
	events, response, err := c.api.GetJson(ctx).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	result := make([]auditEvent, 0, len(events))
	for _, e := range events {
		var timestamp *int64
		if e.Timestamp != nil {
			millis := int64(*e.Timestamp)
			timestamp = &millis
		}
		result = append(result, auditEvent{
			Service:                  e.Service,
			Audit:                    e.Audit,
			AuditType:                e.AuditType,
			Timestamp:                timestamp,
			UserID:                   e.UserId,
			Entity:                   e.Entity,
			EntityID:                 e.EntityId,
			ParentEntity:             e.GetParentEntity(),
			ParentEntityID:           e.GetParentEntityId(),
			EntityAttribute:          e.GetEntityAttribute(),
			EntityAttributeValueFrom: e.GetEntityAttributeValueFrom(),
			EntityAttributeValueTo:   e.GetEntityAttributeValueTo(),
		})
	}
	return result, nil
}
