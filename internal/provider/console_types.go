package provider

import "time"

// The values consoleClient returns and accepts. They carry only what the
// provider uses, in plain Go types, so the generated API types stay inside
// console_client.go.

// cluster is a cluster as the Console reports it. CreateCluster ignores the
// fields the Console only reports.
type cluster struct {
	ID           string
	Name         string
	Description  string // empty when the cluster has none
	ChannelID    string
	RegionID     string
	PlanTypeID   string
	GenerationID string
	AutoUpdate   bool

	// BackupRegionID is empty when the cluster has none.
	BackupRegionID string
	// Encryption is the encryption key type, such as "Provider". CreateCluster
	// sends it only when set.
	Encryption string
	// StageLabel and HardwarePackages are only sent, never reported: the
	// Console doesn't return them. Empty and 0 mean unset.
	StageLabel       string
	HardwarePackages int64

	// Reported by the Console only.
	OwnerID          string
	Created          time.Time
	Labels           []string
	IPAllowlist      []allowlistEntry
	Status           string
	EncryptionStatus string // empty unless the Console reports one
	// The key ARNs are empty until keys were supplied to an "External" cluster.
	EncryptionPrimaryKeyARN   string
	EncryptionSecondaryKeyARN string
	Links                     clusterLinks
}

// clusterLinks are the URLs of a cluster's components; empty when not reported.
type clusterLinks struct {
	Zeebe      string
	Operate    string
	Tasklist   string
	Optimize   string
	Connectors string
	Identity   string
	Console    string
	OAuth      string
}

// clusterUpdate is what UpdateCluster changes in place. An empty Description
// clears it; an empty StageLabel and a zero HardwarePackages leave the
// attribute as it is.
type clusterUpdate struct {
	Name             string
	Description      string
	StageLabel       string
	HardwarePackages int64
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
	Name  string
	// InvitePending is set while the member hasn't accepted the invitation.
	InvitePending bool
	// Roles holds only the roles the API can assign.
	Roles []string
	// AllRoles holds every role the member has, including those the API can't
	// assign, such as owner.
	AllRoles []string
	// Owner is set for the organization owner, whom the API can't change.
	Owner bool
}

// parameters are the channels, regions and plan types clusters can use.
type parameters struct {
	Channels  []channel
	Regions   []namedID
	PlanTypes []namedID
}

// channel is a release channel and the generations a cluster can use in it.
type channel struct {
	ID                 string
	Name               string
	DefaultGeneration  namedID
	AllowedGenerations []namedID
}

// namedID is anything the Console identifies by ID and shows by name.
type namedID struct {
	ID   string
	Name string
}

// ipRanges are the IP ranges Camunda's services connect from.
type ipRanges struct {
	WebModeler []string
}

// auditEvent is one entry of the organization's activity log. Optional
// fields are empty when the Console doesn't report them.
type auditEvent struct {
	Service string
	Audit   string
	// AuditType is the kind of action the Console reports: "c" (create), "r"
	// (read), "u" (update) or "d" (delete).
	AuditType string
	// Timestamp is in milliseconds since the Unix epoch; nil when the Console
	// doesn't report one.
	Timestamp                *int64
	UserID                   string
	Entity                   string
	EntityID                 string
	ParentEntity             string
	ParentEntityID           string
	EntityAttribute          string
	EntityAttributeValueFrom string
	EntityAttributeValueTo   string
}

// backup is a cluster backup. The statuses are the Console's: "In progress",
// "Failed", "Complete" or "-".
type backup struct {
	ID             string
	Name           string
	Created        string
	Completed      string // empty while the backup is running
	Status         string
	ZeebeStatus    string
	OperateStatus  string
	TasklistStatus string
	OptimizeStatus string
	GenerationID   string // empty for legacy backups
	GenerationName string
}
