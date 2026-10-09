package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	console "github.com/camunda-community-hub/console-customer-api-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// fakeConsole serves the OAuth token endpoint. Tests either register the
// Console API routes they need on mux, or call serveConsole for an in-memory
// Console that behaves like the real one.
type fakeConsole struct {
	*httptest.Server
	mux          *http.ServeMux
	tokensIssued atomic.Int32
}

func newFakeConsole(t *testing.T) *fakeConsole {
	t.Helper()

	f := &fakeConsole{mux: http.NewServeMux()}
	f.mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_, secret, ok := r.BasicAuth()
		if !ok {
			secret = r.FormValue("client_secret")
		}
		if secret != "secret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		n := f.tokensIssued.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// expires_in below oauth2's expiry delta forces a refresh on every use.
		fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"bearer","expires_in":1}`, n)
	})
	f.Server = httptest.NewServer(f.mux)
	t.Cleanup(f.Close)

	return f
}

// shortenClusterWait makes cluster health polling fast enough for tests.
func shortenClusterWait(c *consoleClient) {
	c.clusterWaitTimeout = 5 * time.Second
	c.clusterWaitDelay = 0
	c.clusterPollInterval = time.Millisecond
}

func (f *fakeConsole) client(t *testing.T) *consoleClient {
	t.Helper()

	client, err := newConsoleClient(context.Background(), consoleClientConfig{
		APIURL:       f.URL,
		TokenURL:     f.URL + "/oauth/token",
		Audience:     "api.cloud.camunda.io",
		ClientID:     "id",
		ClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("newConsoleClient: %v", err)
	}
	shortenClusterWait(client)

	return client
}

// providerFactories runs the provider against this fake.
func (f *fakeConsole) providerFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"camunda": providerserver.NewProtocol6WithError(&CamundaCloudProvider{tuneClient: shortenClusterWait}),
	}
}

// providerConfig is the HCL provider block pointing at this fake.
func (f *fakeConsole) providerConfig() string {
	return fmt.Sprintf(`
provider "camunda" {
  client_id     = "id"
  client_secret = "secret"
  api_url       = %q
  token_url     = %q
}
`, f.URL, f.URL+"/oauth/token")
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func clusterWithStatus(id string, status console.ClusterComponentStatus) console.Cluster {
	return console.Cluster{
		Uuid:   id,
		Name:   "test",
		Status: console.ClusterStatus{Ready: status},
	}
}

// Parameters offered by the in-memory Console.
const (
	fakeChannelID    = "channel-stable"
	fakeGeneration1  = "gen-1"
	fakeGeneration2  = "gen-2"
	fakeTrialPlanID  = "plan-trial"
	fakeProdPlanID   = "plan-production"
	fakeRegionID     = "region-belgium"
	fakeChannelName  = "Stable"
	fakeTrialPlan    = "Trial Package"
	fakeProdPlan     = "Production"
	fakeRegionName   = "Belgium, Europe (europe-west1)"
	fakeZeebeAddress = "zeebe.example.com:443"
)

// consoleState is an in-memory Camunda Console.
type consoleState struct {
	t  *testing.T
	mu sync.Mutex

	nextID   int
	clusters map[string]*console.Cluster
	// Polls left before a new cluster reports healthy.
	creating map[string]int
	// clusterID -> clientID -> client
	clients map[string]map[string]console.CreatedClusterClient
	// clusterID -> name -> value
	secrets map[string]map[string]string
	members map[string]console.Member
}

// serveConsole registers the Console API on the fake and returns its state.
func (f *fakeConsole) serveConsole(t *testing.T) *consoleState {
	s := &consoleState{
		t:        t,
		clusters: map[string]*console.Cluster{},
		creating: map[string]int{},
		clients:  map[string]map[string]console.CreatedClusterClient{},
		secrets:  map[string]map[string]string{},
		members:  map[string]console.Member{},
	}

	routes := map[string]func(http.ResponseWriter, *http.Request){
		"GET /clusters/parameters":                   s.getParameters,
		"POST /clusters":                             s.createCluster,
		"GET /clusters/{id}":                         s.getCluster,
		"PATCH /clusters/{id}":                       s.updateCluster,
		"DELETE /clusters/{id}":                      s.deleteCluster,
		"PUT /clusters/{id}/ipallowlist":             s.updateIPAllowlist,
		"POST /clusters/{id}/clients":                s.createClient,
		"GET /clusters/{id}/clients":                 s.listClients,
		"GET /clusters/{id}/clients/{clientId}":      s.getClient,
		"DELETE /clusters/{id}/clients/{clientId}":   s.deleteClient,
		"POST /clusters/{id}/secrets":                s.createSecret,
		"GET /clusters/{id}/secrets":                 s.getSecrets,
		"DELETE /clusters/{id}/secrets/{secretName}": s.deleteSecret,
		"GET /members":                               s.getMembers,
		"POST /members/{email}":                      s.updateMember,
		"DELETE /members/{email}":                    s.deleteMember,
	}
	for pattern, handler := range routes {
		f.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			defer s.mu.Unlock()
			handler(w, r)
		})
	}

	return s
}

// do runs fn with the state locked, for tests that inspect or change it.
func (s *consoleState) do(fn func(s *consoleState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s)
}

func (s *consoleState) newID(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s-%d", prefix, s.nextID)
}

func (s *consoleState) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, fmt.Sprintf(`{"message":%q}`, err), http.StatusBadRequest)
		return false
	}
	return true
}

func notFound(w http.ResponseWriter, what string) {
	http.Error(w, fmt.Sprintf(`{"message":"%s not found"}`, what), http.StatusNotFound)
}

func (s *consoleState) getParameters(w http.ResponseWriter, r *http.Request) {
	writeJSON(s.t, w, console.Parameters{
		Channels: []console.ParametersChannelsInner{{
			Uuid:              fakeChannelID,
			Name:              fakeChannelName,
			DefaultGeneration: console.ParametersChannelsInnerDefaultGeneration{Uuid: fakeGeneration1, Name: "8.7"},
			AllowedGenerations: []console.ParametersChannelsInnerAllowedGenerationsInner{
				{Uuid: fakeGeneration1, Name: "8.7"},
				{Uuid: fakeGeneration2, Name: "8.8"},
			},
		}},
		ClusterPlanTypes: []console.ParametersChannelsInnerDefaultGeneration{
			{Uuid: fakeTrialPlanID, Name: fakeTrialPlan},
			{Uuid: fakeProdPlanID, Name: fakeProdPlan},
		},
		Regions: []console.ParametersRegionsInner{
			{Uuid: fakeRegionID, Name: fakeRegionName, Provider: "gcp"},
		},
	})
}

func (s *consoleState) createCluster(w http.ResponseWriter, r *http.Request) {
	var req console.CreateClusterRequest
	if !s.decode(w, r, &req) {
		return
	}

	id := s.newID("cluster")
	s.clusters[id] = &console.Cluster{
		Uuid:        id,
		Name:        req.Name,
		Channel:     console.ClusterChannel{Uuid: req.ChannelId},
		Region:      console.ClusterRegion{Uuid: req.RegionId},
		PlanType:    console.ClusterPlanType{Uuid: req.PlanTypeId},
		Generation:  console.ClusterGeneration{Uuid: req.GenerationId},
		AutoUpdate:  req.AutoUpdate != nil && *req.AutoUpdate,
		Description: req.Description,
		Status:      console.ClusterStatus{Ready: console.CLUSTERCOMPONENTSTATUS_CREATING},
	}
	s.creating[id] = 2
	writeJSON(s.t, w, console.CreateCluster200Response{ClusterId: id})
}

func (s *consoleState) getCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cluster, ok := s.clusters[id]
	if !ok {
		notFound(w, "cluster")
		return
	}

	if s.creating[id] > 0 {
		s.creating[id]--
	} else {
		cluster.Status.Ready = console.CLUSTERCOMPONENTSTATUS_HEALTHY
	}
	writeJSON(s.t, w, cluster)
}

func (s *consoleState) updateCluster(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.clusters[r.PathValue("id")]
	if !ok {
		notFound(w, "cluster")
		return
	}

	var req console.UpdateClusterBody
	if !s.decode(w, r, &req) {
		return
	}
	if req.Name != nil {
		cluster.Name = *req.Name
	}
	if req.Description != nil {
		cluster.Description = req.Description
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) deleteCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.clusters[id]; !ok {
		notFound(w, "cluster")
		return
	}
	delete(s.clusters, id)
	delete(s.clients, id)
	delete(s.secrets, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) updateIPAllowlist(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.clusters[r.PathValue("id")]
	if !ok {
		notFound(w, "cluster")
		return
	}

	var req console.IpAllowListBody
	if !s.decode(w, r, &req) {
		return
	}
	cluster.Ipallowlist = req.Ipallowlist
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) createClient(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.clusters[clusterID]; !ok {
		notFound(w, "cluster")
		return
	}

	var req console.CreateClusterClientBody
	if !s.decode(w, r, &req) {
		return
	}

	client := console.CreatedClusterClient{
		Uuid:         s.newID("uuid"),
		ClientId:     s.newID("client"),
		ClientSecret: s.newID("client-secret"),
		Name:         req.ClientName,
		Permissions:  req.Permissions,
	}
	if s.clients[clusterID] == nil {
		s.clients[clusterID] = map[string]console.CreatedClusterClient{}
	}
	s.clients[clusterID][client.ClientId] = client
	writeJSON(s.t, w, client)
}

func (s *consoleState) listClients(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.clusters[clusterID]; !ok {
		notFound(w, "cluster")
		return
	}

	clients := []console.ClusterClient{}
	for _, client := range s.clients[clusterID] {
		clients = append(clients, console.ClusterClient{
			ClientId:    client.ClientId,
			Name:        client.Name,
			Permissions: client.Permissions,
		})
	}
	writeJSON(s.t, w, clients)
}

func (s *consoleState) getClient(w http.ResponseWriter, r *http.Request) {
	client, ok := s.clients[r.PathValue("id")][r.PathValue("clientId")]
	if !ok {
		notFound(w, "client")
		return
	}

	writeJSON(s.t, w, console.ClusterClientConnectionDetails{
		Name:                           client.Name,
		ZEEBE_CLIENT_ID:                client.ClientId,
		ZEEBE_ADDRESS:                  fakeZeebeAddress,
		ZEEBE_AUTHORIZATION_SERVER_URL: "https://login.example.com/oauth/token",
	})
}

func (s *consoleState) deleteClient(w http.ResponseWriter, r *http.Request) {
	clusterID, clientID := r.PathValue("id"), r.PathValue("clientId")
	if _, ok := s.clients[clusterID][clientID]; !ok {
		notFound(w, "client")
		return
	}
	delete(s.clients[clusterID], clientID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) createSecret(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.clusters[clusterID]; !ok {
		notFound(w, "cluster")
		return
	}

	var req console.CreateSecretBody
	if !s.decode(w, r, &req) {
		return
	}
	if s.secrets[clusterID] == nil {
		s.secrets[clusterID] = map[string]string{}
	}
	s.secrets[clusterID][req.SecretName] = req.SecretValue
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) getSecrets(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.clusters[clusterID]; !ok {
		notFound(w, "cluster")
		return
	}

	secrets := s.secrets[clusterID]
	if secrets == nil {
		secrets = map[string]string{}
	}
	writeJSON(s.t, w, secrets)
}

func (s *consoleState) deleteSecret(w http.ResponseWriter, r *http.Request) {
	clusterID, name := r.PathValue("id"), r.PathValue("secretName")
	if _, ok := s.secrets[clusterID][name]; !ok {
		notFound(w, "secret")
		return
	}
	delete(s.secrets[clusterID], name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) getMembers(w http.ResponseWriter, r *http.Request) {
	members := []console.Member{}
	for _, member := range s.members {
		members = append(members, member)
	}
	writeJSON(s.t, w, members)
}

func (s *consoleState) updateMember(w http.ResponseWriter, r *http.Request) {
	var req console.PostMemberBody
	if !s.decode(w, r, &req) {
		return
	}

	email := r.PathValue("email")
	member := console.Member{Email: email, Name: email, Roles: []console.OrganizationRole{}}
	for _, role := range req.OrgRoles {
		member.Roles = append(member.Roles, console.OrganizationRole(role))
	}
	s.members[email] = member
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) deleteMember(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if _, ok := s.members[email]; !ok {
		notFound(w, "member")
		return
	}
	delete(s.members, email)
	w.WriteHeader(http.StatusNoContent)
}
