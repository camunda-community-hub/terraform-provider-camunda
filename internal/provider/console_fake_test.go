package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
		if _, err := fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"bearer","expires_in":1}`, n); err != nil {
			t.Errorf("write token response: %v", err)
		}
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
	fakeChannelID   = "channel-stable"
	fakeGeneration1 = "gen-1"
	fakeGeneration2 = "gen-2"
	fakeTrialPlanID = "plan-trial"
	fakeProdPlanID  = "plan-production"
	fakeRegionID    = "region-belgium"
	fakeChannelName = "Stable"
	fakeTrialPlan   = "Trial Package"
	fakeProdPlan    = "Production"
	fakeRegionName  = "Belgium, Europe (europe-west1)"
	// AWS regions, which customer-managed encryption needs.
	fakeAWSRegionID     = "region-frankfurt"
	fakeAWSRegionName   = "Frankfurt, Europe (eu-central-1)"
	fakeAWSBackupRegion = "region-ireland"
	fakeAWSBackupName   = "Ireland, Europe (eu-west-1)"
	fakeZeebeAddress    = "zeebe.example.com:443"
)

// clusterSettings are the cluster attributes the Console accepts but never
// reports.
type clusterSettings struct {
	stageLabel       console.CamundaClusterStage
	hardwarePackages float64
}

// fakeSecureConnectivity is an activated secure connectivity; its endpoint
// only exists after a few polls.
type fakeSecureConnectivity struct {
	principals, regions []string
	pollsLeft           int
}

// consoleState is an in-memory Camunda Console.
type consoleState struct {
	t  *testing.T
	mu sync.Mutex

	nextID   int
	clusters map[string]*console.Cluster
	// What the Console accepts for a cluster but doesn't report back.
	clusterSettings map[string]*clusterSettings
	// Polls left before a new cluster reports healthy.
	creating map[string]int
	// clusterID -> clientID -> client
	clients map[string]map[string]console.CreatedClusterClient
	// clusterID -> name -> value
	secrets map[string]map[string]string
	// clusterID -> backups, oldest first
	backups map[string][]console.BackupDto
	// Polls left before a new backup stops being in progress. It then reports
	// failed instead of complete when failBackups is set.
	backupPolls map[string]int
	// clusterID -> cron expression
	schedules map[string]string
	// backupID -> restore state; absent when never restored
	restores map[string]console.RestoreState
	// backupID -> UUID of its latest restore
	restoreIDs   map[string]string
	failRestores bool
	// supersedeRestores makes another restore of the same backup replace the one a caller started.
	supersedeRestores bool
	// clusterID -> failover request received
	failovers map[string]console.TriggerFailoverBody
	// clusterID -> polls left before BYOK encryption is ready
	encryptionPolls map[string]int
	// slowEncryption keeps newly supplied keys from ever becoming ready.
	slowEncryption bool
	// clusterID -> polls left before external monitoring is active; absent when it is not activated
	monitoringPolls map[string]int
	// slowMonitoring keeps new external monitoring provisioning for good.
	slowMonitoring bool
	// clusterID -> monitoring clients, and their current password by uuid
	monitoringClients   map[string][]console.ByomClientDto
	monitoringPasswords map[string]string
	// clusterID -> secure connectivity activated for the cluster
	secureConnectivity map[string]*fakeSecureConnectivity
	// slowSecureConnectivity keeps new secure connectivity provisioning for good.
	slowSecureConnectivity bool
	failBackups            bool
	members                map[string]console.Member
}

// serveConsole registers the Console API on the fake and returns its state.
func (f *fakeConsole) serveConsole(t *testing.T) *consoleState {
	s := &consoleState{
		t:                   t,
		clusters:            map[string]*console.Cluster{},
		clusterSettings:     map[string]*clusterSettings{},
		creating:            map[string]int{},
		clients:             map[string]map[string]console.CreatedClusterClient{},
		secrets:             map[string]map[string]string{},
		backups:             map[string][]console.BackupDto{},
		backupPolls:         map[string]int{},
		schedules:           map[string]string{},
		restores:            map[string]console.RestoreState{},
		restoreIDs:          map[string]string{},
		failovers:           map[string]console.TriggerFailoverBody{},
		encryptionPolls:     map[string]int{},
		monitoringPolls:     map[string]int{},
		monitoringClients:   map[string][]console.ByomClientDto{},
		monitoringPasswords: map[string]string{},
		secureConnectivity:  map[string]*fakeSecureConnectivity{},
		members:             map[string]console.Member{},
	}

	routes := map[string]func(http.ResponseWriter, *http.Request){
		"PUT /clusters/{id}/upgrade":                               s.upgradeCluster,
		"PUT /clusters/{id}/wake":                                  s.wakeCluster,
		"POST /clusters/{id}/backups/{backupId}/restore":           s.restoreBackup,
		"GET /clusters/{id}/backups/{backupId}/restore":            s.getRestore,
		"POST /clusters/{id}/failover":                             s.triggerFailover,
		"PUT /clusters/{id}/encryption":                            s.updateClusterEncryption,
		"POST /clusters/{id}/monitoring":                           s.activateMonitoring,
		"DELETE /clusters/{id}/monitoring":                         s.deactivateMonitoring,
		"GET /clusters/{id}/monitoring/clients":                    s.listMonitoringClients,
		"POST /clusters/{id}/monitoring/clients":                   s.createMonitoringClient,
		"POST /clusters/{id}/monitoring/clients/{clientId}/rotate": s.rotateMonitoringClientPassword,
		"DELETE /clusters/{id}/monitoring/clients/{clientId}":      s.deleteMonitoringClient,
		"POST /clusters/{id}/secure-connectivity":                  s.activateSecureConnectivity,
		"GET /clusters/{id}/secure-connectivity":                   s.getSecureConnectivity,
		"DELETE /clusters/{id}/secure-connectivity":                s.deactivateSecureConnectivity,
		"GET /clusters/{id}/backups/schedule":                      s.getSchedule,
		"PUT /clusters/{id}/backups/schedule":                      s.putSchedule,
		"DELETE /clusters/{id}/backups/schedule":                   s.deleteSchedule,
		"GET /clusters/parameters":                                 s.getParameters,
		"GET /meta/ip-ranges":                                      s.getIPRanges,
		"GET /activity/json":                                       s.getAuditLog,
		"POST /clusters":                                           s.createCluster,
		"GET /clusters":                                            s.listClusters,
		"GET /clusters/{id}":                                       s.getCluster,
		"PATCH /clusters/{id}":                                     s.updateCluster,
		"DELETE /clusters/{id}":                                    s.deleteCluster,
		"PUT /clusters/{id}/ipallowlist":                           s.updateIPAllowlist,
		"POST /clusters/{id}/clients":                              s.createClient,
		"GET /clusters/{id}/clients":                               s.listClients,
		"GET /clusters/{id}/clients/{clientId}":                    s.getClient,
		"DELETE /clusters/{id}/clients/{clientId}":                 s.deleteClient,
		"POST /clusters/{id}/secrets":                              s.createSecret,
		"GET /clusters/{id}/secrets":                               s.getSecrets,
		"DELETE /clusters/{id}/secrets/{secretName}":               s.deleteSecret,
		"GET /clusters/{id}/backups":                               s.listBackups,
		"POST /clusters/{id}/backups":                              s.createBackup,
		"DELETE /clusters/{id}/backups/{backupId}":                 s.deleteBackup,
		"GET /members":                                             s.getMembers,
		"POST /members/{email}":                                    s.updateMember,
		"DELETE /members/{email}":                                  s.deleteMember,
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

// cluster returns the cluster, or answers 404 and reports false. Handlers for
// cluster-scoped endpoints start with it.
func (s *consoleState) cluster(w http.ResponseWriter, id string) (*console.Cluster, bool) {
	cluster, ok := s.clusters[id]
	if !ok {
		notFound(w, "cluster")
	}
	return cluster, ok
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
			{Uuid: fakeAWSRegionID, Name: fakeAWSRegionName, Provider: "aws"},
			{Uuid: fakeAWSBackupRegion, Name: fakeAWSBackupName, Provider: "aws"},
		},
	})
}

func (s *consoleState) getIPRanges(w http.ResponseWriter, r *http.Request) {
	writeJSON(s.t, w, console.MetaDto{WebModeler: []string{"10.0.0.0/24", "10.0.1.0/24"}})
}

func (s *consoleState) getAuditLog(w http.ResponseWriter, r *http.Request) {
	timestamp := 1700000000000.0
	parent, parentID := "cluster", "c1"
	writeJSON(s.t, w, []console.AuditDto{
		{Service: "console", OrgId: "org", Audit: "Cluster created", AuditType: "c", Entity: "cluster", EntityId: "c1", UserId: "u1", Timestamp: &timestamp},
		{Service: "console", OrgId: "org", Audit: "Client created", AuditType: "c", Entity: "client", EntityId: "k1", ParentEntity: &parent, ParentEntityId: &parentID, UserId: "u2"},
	})
}

func (s *consoleState) createCluster(w http.ResponseWriter, r *http.Request) {
	var req console.CreateClusterRequest
	if !s.decode(w, r, &req) {
		return
	}

	id := s.newID("cluster")
	settings := &clusterSettings{}
	if req.StageLabel != nil {
		settings.stageLabel = *req.StageLabel
	}
	if req.HardwarePackages != nil {
		settings.hardwarePackages = *req.HardwarePackages
	}
	s.clusterSettings[id] = settings
	zeebeLink := fmt.Sprintf("https://%s.zeebe.example.com", id)
	s.clusters[id] = &console.Cluster{
		OwnerId:     "owner-1",
		Links:       console.ClusterLinks{Zeebe: &zeebeLink},
		Encryption:  &console.ClusterEncryption{Type: console.CLUSTERENCRYPTIONKEY_PROVIDER},
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
	cluster := s.clusters[id]
	if req.Encryption != nil {
		cluster.Encryption.Type = *req.Encryption
		if *req.Encryption == console.CLUSTERENCRYPTIONKEY_EXTERNAL {
			status := console.ENCRYPTIONSTATUS_EXTERNAL_ENCRYPTION_KEY_NOT_SUPPLIED
			cluster.Encryption.Status = &status
		}
	}
	if req.BackupRegionId != nil {
		cluster.BackupRegion = &console.ClusterBackupRegion{Uuid: *req.BackupRegionId}
	}
	s.creating[id] = 2
	writeJSON(s.t, w, console.CreateCluster200Response{ClusterId: id})
}

func (s *consoleState) listClusters(w http.ResponseWriter, r *http.Request) {
	clusters := []console.Cluster{}
	for _, cluster := range s.clusters {
		clusters = append(clusters, *cluster)
	}
	slices.SortFunc(clusters, func(a, b console.Cluster) int { return strings.Compare(a.Uuid, b.Uuid) })
	writeJSON(s.t, w, clusters)
}

func (s *consoleState) getCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cluster, ok := s.cluster(w, id)
	if !ok {
		return
	}

	if s.creating[id] > 0 {
		s.creating[id]--
	} else if e := cluster.Encryption; e != nil && e.Type == console.CLUSTERENCRYPTIONKEY_EXTERNAL && e.PrimaryKeyArn == nil {
		// The cluster waits for its external keys.
		cluster.Status.Ready = console.CLUSTERCOMPONENTSTATUS_WAITING_FOR_INPUT
	} else {
		cluster.Status.Ready = console.CLUSTERCOMPONENTSTATUS_HEALTHY
	}
	if e := cluster.Encryption; e != nil && e.Status != nil && *e.Status == console.ENCRYPTIONSTATUS_EXTERNAL_ENCRYPTION_KEY_NOT_READY {
		if s.encryptionPolls[id] > 0 {
			s.encryptionPolls[id]--
		} else {
			ready := console.ENCRYPTIONSTATUS_READY
			e.Status = &ready
		}
	}
	writeJSON(s.t, w, cluster)
}

func (s *consoleState) updateCluster(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.cluster(w, r.PathValue("id"))
	if !ok {
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
	settings := s.clusterSettings[cluster.Uuid]
	if req.StageLabel != nil {
		settings.stageLabel = *req.StageLabel
	}
	if req.NumberOfAllocatedHwPackages != nil {
		settings.hardwarePackages = *req.NumberOfAllocatedHwPackages
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) deleteCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.cluster(w, id); !ok {
		return
	}
	delete(s.clusters, id)
	delete(s.clusterSettings, id)
	delete(s.clients, id)
	delete(s.secrets, id)
	delete(s.backups, id)
	delete(s.schedules, id)
	delete(s.monitoringPolls, id)
	delete(s.monitoringClients, id)
	delete(s.secureConnectivity, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) updateIPAllowlist(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.cluster(w, r.PathValue("id"))
	if !ok {
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
	if _, ok := s.cluster(w, clusterID); !ok {
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
	if _, ok := s.cluster(w, clusterID); !ok {
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
	if _, ok := s.cluster(w, clusterID); !ok {
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
	if _, ok := s.cluster(w, clusterID); !ok {
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

func (s *consoleState) listBackups(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	backups := s.backups[clusterID]
	for i := range backups {
		backup := &backups[i]
		if backup.Status != console.BACKUPSTATUS_IN_PROGRESS {
			continue
		}
		if s.backupPolls[backup.Uuid] > 0 {
			s.backupPolls[backup.Uuid]--
			continue
		}
		status := console.BACKUPSTATUS_COMPLETE
		if s.failBackups {
			status = console.BACKUPSTATUS_FAILED
		}
		setBackupStatus(backup, status)
	}
	if backups == nil {
		backups = []console.BackupDto{}
	}
	writeJSON(s.t, w, backups)
}

func setBackupStatus(backup *console.BackupDto, status console.BackupStatus) {
	backup.Status = status
	backup.ZeebeStatus = status
	backup.OperateStatus = status
	backup.TasklistStatus = status
	backup.OptimizeStatus = status
	if status == console.BACKUPSTATUS_COMPLETE {
		backup.Completed = "2026-01-01T00:10:00Z"
	}
}

func (s *consoleState) createBackup(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	id := s.newID("backup")
	backup := console.BackupDto{Uuid: id, Name: id, Created: "2026-01-01T00:00:00Z"}
	setBackupStatus(&backup, console.BACKUPSTATUS_IN_PROGRESS)
	s.backups[clusterID] = append(s.backups[clusterID], backup)
	s.backupPolls[id] = 2
	writeJSON(s.t, w, backup)
}

func (s *consoleState) deleteBackup(w http.ResponseWriter, r *http.Request) {
	clusterID, backupID := r.PathValue("id"), r.PathValue("backupId")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	backups := s.backups[clusterID]
	index := slices.IndexFunc(backups, func(b console.BackupDto) bool { return b.Uuid == backupID })
	if index < 0 {
		notFound(w, "backup")
		return
	}
	deleted := backups[index]
	s.backups[clusterID] = slices.Delete(backups, index, index+1)
	writeJSON(s.t, w, deleted)
}

func (s *consoleState) getSchedule(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	schedule, ok := s.schedules[clusterID]
	if !ok {
		notFound(w, "backup schedule")
		return
	}
	writeJSON(s.t, w, console.BackupScheduleDto{Name: "schedule-" + clusterID, Schedule: schedule})
}

func (s *consoleState) putSchedule(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	var req console.BackupScheduleBody
	if !s.decode(w, r, &req) {
		return
	}
	if len(strings.Fields(req.Schedule)) != 5 {
		http.Error(w, `{"message":"not a valid cron expression"}`, http.StatusBadRequest)
		return
	}
	s.schedules[clusterID] = req.Schedule
	writeJSON(s.t, w, console.BackupScheduleDto{Name: "schedule-" + clusterID, Schedule: req.Schedule})
}

func (s *consoleState) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}
	delete(s.schedules, clusterID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) activateSecureConnectivity(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}
	if s.secureConnectivity[clusterID] != nil {
		http.Error(w, `{"message":"already activated"}`, http.StatusConflict)
		return
	}

	var req console.ActivateSecureConnectivityRequest
	if !s.decode(w, r, &req) {
		return
	}
	s.secureConnectivity[clusterID] = &fakeSecureConnectivity{
		principals: req.AllowedPrincipals,
		regions:    req.AllowedRegions,
		pollsLeft:  2,
	}
	if s.slowSecureConnectivity {
		s.secureConnectivity[clusterID].pollsLeft = 1 << 20
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) getSecureConnectivity(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	active := s.secureConnectivity[clusterID]
	if active == nil || active.pollsLeft > 0 {
		if active != nil {
			active.pollsLeft--
		}
		writeJSON(s.t, w, map[string]any{"status": map[string]any{}})
		return
	}

	writeJSON(s.t, w, map[string]any{"status": map[string]any{
		"metadata": map[string]any{"name": "sc-" + clusterID, "namespace": "ns", "labels": map[string]any{"orgId": "org"}},
		"spec": map[string]any{
			"allowedPrincipals": active.principals,
			"allowedRegions":    active.regions,
			"cluster":           map[string]any{"id": clusterID},
		},
		"status": map[string]any{
			"conditions": []map[string]any{{
				"type": "Ready", "status": "True", "reason": "EndpointReady", "message": "ready",
				"lastTransitionTime": "2026-01-01T00:00:00Z", "observedGeneration": 1,
			}},
			"endpointConnections": []map[string]any{{
				"state": "available", "serviceId": "vpce-svc-1", "owner": "123456789012",
				"endpointId": "vpce-1", "creationTimestamp": "2026-01-02T00:00:00Z",
			}},
			"endpoint": map[string]any{
				"privateDnsName": "sc.internal", "region": "eu-west-1", "serviceName": "com.amazonaws.vpce.sc", "type": "Interface",
			},
			"endpointConnectionCount": 3,
			"observedGeneration":      1,
			"urls":                    map[string]any{},
		},
	}})
}

func (s *consoleState) deactivateSecureConnectivity(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}
	if s.secureConnectivity[clusterID] == nil {
		http.Error(w, `{"message":"not activated"}`, http.StatusConflict)
		return
	}
	delete(s.secureConnectivity, clusterID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) activateMonitoring(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}
	if _, active := s.monitoringPolls[clusterID]; active {
		http.Error(w, `{"message":"already activated"}`, http.StatusConflict)
		return
	}
	s.monitoringPolls[clusterID] = 2
	if s.slowMonitoring {
		s.monitoringPolls[clusterID] = 1 << 20
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) deactivateMonitoring(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}
	if _, active := s.monitoringPolls[clusterID]; !active {
		http.Error(w, `{"message":"not activated"}`, http.StatusConflict)
		return
	}
	delete(s.monitoringPolls, clusterID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) listMonitoringClients(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	clients := s.monitoringClients[clusterID]
	if clients == nil {
		clients = []console.ByomClientDto{}
	}
	status := map[string]any{}
	if polls, active := s.monitoringPolls[clusterID]; active {
		if polls > 0 {
			s.monitoringPolls[clusterID]--
		} else {
			status = map[string]any{
				"metricsEndpoint": map[string]any{"target": "metrics.example.com", "scheme": "https", "path": "/metrics"},
				"conditions":      []any{map[string]any{"type": "Ready", "status": "True"}},
			}
		}
	}
	writeJSON(s.t, w, map[string]any{"clients": clients, "status": status})
}

func (s *consoleState) createMonitoringClient(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	var req console.CreateMonitoringClientRequest
	if !s.decode(w, r, &req) {
		return
	}
	id := s.newID("monitoring-client")
	client := console.ByomClientDto{
		Uuid: id, Name: req.Username, Username: req.Username,
		Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), LastUsed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: "u1", CreatedByName: "User One",
	}
	s.monitoringClients[clusterID] = append(s.monitoringClients[clusterID], client)
	s.monitoringPasswords[id] = s.newID("password")
	s.writeMonitoringClient(w, client)
}

func (s *consoleState) rotateMonitoringClientPassword(w http.ResponseWriter, r *http.Request) {
	clusterID, clientID := r.PathValue("id"), r.PathValue("clientId")
	for _, client := range s.monitoringClients[clusterID] {
		if client.Uuid == clientID {
			s.monitoringPasswords[clientID] = s.newID("password")
			s.writeMonitoringClient(w, client)
			return
		}
	}
	notFound(w, "monitoring client")
}

func (s *consoleState) deleteMonitoringClient(w http.ResponseWriter, r *http.Request) {
	clusterID, clientID := r.PathValue("id"), r.PathValue("clientId")
	clients := s.monitoringClients[clusterID]
	index := slices.IndexFunc(clients, func(c console.ByomClientDto) bool { return c.Uuid == clientID })
	if index < 0 {
		notFound(w, "monitoring client")
		return
	}
	s.monitoringClients[clusterID] = slices.Delete(clients, index, index+1)
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) writeMonitoringClient(w http.ResponseWriter, client console.ByomClientDto) {
	writeJSON(s.t, w, console.CreateMonitoringClient200Response{
		Uuid: client.Uuid, Name: client.Name, Username: client.Username,
		Created: client.Created, LastUsed: client.LastUsed,
		CreatedBy: client.CreatedBy, CreatedByName: client.CreatedByName,
		Password: s.monitoringPasswords[client.Uuid],
	})
}

func (s *consoleState) updateClusterEncryption(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.cluster(w, r.PathValue("id"))
	if !ok {
		return
	}
	if cluster.Encryption == nil || cluster.Encryption.Type != console.CLUSTERENCRYPTIONKEY_EXTERNAL {
		http.Error(w, `{"message":"cluster is not configured for external encryption"}`, http.StatusConflict)
		return
	}

	if cluster.Encryption.PrimaryKeyArn != nil {
		http.Error(w, `{"message":"keys are already applied and can't be changed"}`, http.StatusConflict)
		return
	}

	var req console.UpdateClusterEncryptionBody
	if !s.decode(w, r, &req) {
		return
	}
	// The key must live in the cluster's AWS region and, for a dual-region
	// cluster only, a second one in its backup region.
	awsRegions := map[string]string{fakeAWSRegionID: "eu-central-1", fakeAWSBackupRegion: "eu-west-1"}
	primaryRegion, aws := awsRegions[cluster.Region.Uuid]
	if !aws {
		http.Error(w, `{"message":"external encryption is only available on AWS"}`, http.StatusForbidden)
		return
	}
	if keyRegion(req.PrimaryEncryptionKeyId) != primaryRegion {
		http.Error(w, `{"message":"primary key ARN is invalid or not in the cluster's region"}`, http.StatusBadRequest)
		return
	}
	switch {
	case cluster.BackupRegion != nil:
		if req.SecondaryEncryptionKeyId == nil || *req.SecondaryEncryptionKeyId == req.PrimaryEncryptionKeyId ||
			keyRegion(*req.SecondaryEncryptionKeyId) != awsRegions[cluster.BackupRegion.Uuid] {
			http.Error(w, `{"message":"a dual-region cluster needs a distinct secondary key in its backup region"}`, http.StatusBadRequest)
			return
		}
	case req.SecondaryEncryptionKeyId != nil:
		http.Error(w, `{"message":"a single-region cluster takes no secondary key"}`, http.StatusBadRequest)
		return
	}
	cluster.Encryption.PrimaryKeyArn = &req.PrimaryEncryptionKeyId
	cluster.Encryption.SecondaryKeyArn = req.SecondaryEncryptionKeyId
	notReady := console.ENCRYPTIONSTATUS_EXTERNAL_ENCRYPTION_KEY_NOT_READY
	cluster.Encryption.Status = &notReady
	s.encryptionPolls[cluster.Uuid] = 2
	if s.slowEncryption {
		s.encryptionPolls[cluster.Uuid] = 1 << 20
	}
	writeJSON(s.t, w, cluster)
}

func (s *consoleState) upgradeCluster(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.cluster(w, r.PathValue("id"))
	if !ok {
		return
	}
	if cluster.Generation.Uuid == fakeGeneration2 {
		http.Error(w, `{"message":"no upgrade available"}`, http.StatusBadRequest)
		return
	}

	cluster.Generation = console.ClusterGeneration{Uuid: fakeGeneration2}
	cluster.Status.Ready = console.CLUSTERCOMPONENTSTATUS_UPDATING
	s.creating[cluster.Uuid] = 2
	writeJSON(s.t, w, console.GenerationUpgradeForClusterDto{
		Cluster:       console.GenerationUpgradeForClusterDtoCluster{Id: cluster.Uuid, Name: cluster.Name},
		OldGeneration: console.GenerationUpgradeForClusterDtoCluster{Id: fakeGeneration1, Name: "8.7"},
		NewGeneration: console.GenerationUpgradeForClusterDtoCluster{Id: fakeGeneration2, Name: "8.8"},
		OrgId:         "org",
	})
}

func (s *consoleState) wakeCluster(w http.ResponseWriter, r *http.Request) {
	cluster, ok := s.cluster(w, r.PathValue("id"))
	if !ok {
		return
	}
	cluster.Status.Ready = console.CLUSTERCOMPONENTSTATUS_RESUMING
	s.creating[cluster.Uuid] = 2
	w.WriteHeader(http.StatusNoContent)
}

func (s *consoleState) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cluster(w, r.PathValue("id")); !ok {
		return
	}
	backupID := r.PathValue("backupId")
	if state, restoring := s.restores[backupID]; restoring && state != console.RESTORESTATE_COMPLETED && state != console.RESTORESTATE_FAILED {
		http.Error(w, `{"message":"restore in flight"}`, http.StatusConflict)
		return
	}
	s.restores[backupID] = console.RESTORESTATE_PENDING
	s.restoreIDs[backupID] = s.newID("restore")
	writeJSON(s.t, w, console.RestoreDto{Uuid: s.restoreIDs[backupID], BackupId: backupID, State: console.RESTORESTATE_PENDING})
}

func (s *consoleState) getRestore(w http.ResponseWriter, r *http.Request) {
	backupID := r.PathValue("backupId")
	state, ok := s.restores[backupID]
	if !ok {
		notFound(w, "restore")
		return
	}

	// Each read moves the restore along.
	switch state {
	case console.RESTORESTATE_PENDING:
		state = console.RESTORESTATE_IN_PROGRESS
	case console.RESTORESTATE_IN_PROGRESS:
		state = console.RESTORESTATE_COMPLETED
		if s.failRestores {
			state = console.RESTORESTATE_FAILED
		}
	}
	s.restores[backupID] = state
	id := s.restoreIDs[backupID]
	if s.supersedeRestores {
		id = "someone-elses"
	}
	writeJSON(s.t, w, console.RestoreDto{Uuid: id, BackupId: backupID, State: state})
}

func (s *consoleState) triggerFailover(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, ok := s.cluster(w, clusterID); !ok {
		return
	}

	var req console.TriggerFailoverBody
	if !s.decode(w, r, &req) {
		return
	}
	s.failovers[clusterID] = req
	writeJSON(s.t, w, map[string]any{
		"newClusterId": "failover-cluster",
		"results":      map[string]any{"backupClone": "ok", "restore": "ok"},
	})
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
	if s.isOwner(email) {
		http.Error(w, `{"message":"the organization owner can't be changed"}`, http.StatusBadRequest)
		return
	}
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
	if s.isOwner(email) {
		http.Error(w, `{"message":"the organization owner can't be removed"}`, http.StatusBadRequest)
		return
	}
	delete(s.members, email)
	w.WriteHeader(http.StatusNoContent)
}

// isOwner reports whether the member is the organization owner, whom the fake,
// like the real API, refuses to change or remove.
func (s *consoleState) isOwner(email string) bool {
	return slices.Contains(s.members[email].Roles, console.ORGANIZATIONROLE_OWNER)
}

// keyRegion is the region segment of an AWS KMS key ARN, or "" if it isn't one.
func keyRegion(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "kms" {
		return ""
	}
	return parts[3]
}
