package serverpages

type DisplayServer struct {
	ID                  string
	Name                string
	Address             string
	ConnectionType      string
	CredentialReference *string
}

type DisplayInput struct {
	Name                string
	Address             string
	ConnectionType      string
	CredentialReference string
}
