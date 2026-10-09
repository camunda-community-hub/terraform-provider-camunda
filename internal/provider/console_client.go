package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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
	apiCfg.HTTPClient = oauth2.NewClient(context.WithoutCancel(ctx), tokenSource)

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
	return &cluster{
		ID:           found.Uuid,
		Name:         found.Name,
		Description:  found.GetDescription(),
		ChannelID:    found.Channel.Uuid,
		RegionID:     found.Region.Uuid,
		PlanTypeID:   found.PlanType.Uuid,
		GenerationID: found.Generation.Uuid,
		AutoUpdate:   found.AutoUpdate,
	}, nil
}

func (c *consoleClient) getAPICluster(ctx context.Context, clusterID string) (*console.Cluster, error) {
	found, response, err := c.api.GetCluster(ctx, clusterID).Execute()
	return found, apiError(err, response)
}

// UpdateCluster sets the cluster's name and description; an empty description
// clears it.
func (c *consoleClient) UpdateCluster(ctx context.Context, clusterID, name, description string) error {
	response, err := c.api.UpdateCluster(ctx, clusterID).
		UpdateClusterBody(console.UpdateClusterBody{Name: &name, Description: &description}).
		Execute()
	return apiError(err, response)
}

func (c *consoleClient) DeleteCluster(ctx context.Context, clusterID string) error {
	response, err := c.api.DeleteCluster(ctx, clusterID).Execute()
	return apiError(err, response)
}

// WaitClusterHealthy blocks until the cluster reports healthy twice in a row.
func (c *consoleClient) WaitClusterHealthy(ctx context.Context, clusterID string) error {
	wait := &retry.StateChangeConf{
		Pending: []string{
			string(console.CLUSTERCOMPONENTSTATUS_CREATING),
			string(console.CLUSTERCOMPONENTSTATUS_UPDATING),
		},
		Target: []string{
			string(console.CLUSTERCOMPONENTSTATUS_HEALTHY),
		},
		ContinuousTargetOccurence: 2,
		Refresh: func() (interface{}, string, error) {
			cluster, err := c.getAPICluster(ctx, clusterID)
			if err != nil {
				return nil, "", err
			}

			tflog.Info(ctx, "Camunda cluster status", map[string]interface{}{
				"clusterID":     cluster.Uuid,
				"clusterStatus": cluster.Status.Ready,
			})

			return cluster, string(cluster.Status.Ready), nil
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

// GetMember returns the member with the given email, or errNotFound. Roles
// holds only assignable roles; Owner reports the owner role.
func (c *consoleClient) GetMember(ctx context.Context, email string) (*member, error) {
	members, response, err := c.api.GetMembers(ctx).Execute()
	if err != nil {
		return nil, apiError(err, response)
	}

	for _, listed := range members {
		if listed.Email != email {
			continue
		}
		found := &member{Email: listed.Email, Roles: []string{}}
		for _, role := range listed.Roles {
			if role == console.ORGANIZATIONROLE_OWNER {
				found.Owner = true
			}
			if slices.Contains(assignableMemberRoles, string(role)) {
				found.Roles = append(found.Roles, string(role))
			}
		}
		return found, nil
	}
	return nil, fmt.Errorf("%w: organization member %q", errNotFound, email)
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

	response, err := c.api.UpdateMembers(ctx, email).
		PostMemberBody(console.PostMemberBody{OrgRoles: orgRoles}).
		Execute()
	return apiError(err, response)
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
	return apiError(err, response)
}

// Parameters

// GetParameters returns the channels, plan types and regions available to the
// organization.
func (c *consoleClient) GetParameters(ctx context.Context) (*console.Parameters, error) {
	params, response, err := c.api.GetParameters(ctx).Execute()
	return params, apiError(err, response)
}
