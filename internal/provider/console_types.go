package provider

// The values consoleClient returns and accepts. They carry only what the
// provider uses, in plain Go types, so the generated API types stay inside
// console_client.go.

// cluster is a cluster as the Console reports it. CreateCluster ignores ID.
type cluster struct {
	ID           string
	Name         string
	Description  string // empty when the cluster has none
	ChannelID    string
	RegionID     string
	PlanTypeID   string
	GenerationID string
	AutoUpdate   bool
}

// allowlistEntry is one entry of a cluster's IP allowlist.
type allowlistEntry struct {
	IP          string
	Description string
}

// clusterClient is a cluster client as it exists in the Console. The client
// secret is only returned once, by CreateClusterClient.
type clusterClient struct {
	ClientID               string
	Name                   string
	Scopes                 []string
	ZeebeAddress           string
	AuthorizationServerURL string
}

// createdClusterClient is what creating a cluster client returns.
type createdClusterClient struct {
	ClientID string
	Secret   string
}

// member is an organization member.
type member struct {
	Email string
	// Roles holds only the roles the API can assign.
	Roles []string
	// Owner is set for the organization owner, whom the API can't change.
	Owner bool
}
