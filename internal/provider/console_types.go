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
