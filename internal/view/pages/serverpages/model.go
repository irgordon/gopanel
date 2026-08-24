package serverpages

type DisplayServer struct {
	ID             string
	Name           string
	Address        string
	ConnectionType string
}

type DisplayInput struct {
	Name           string
	Address        string
	ConnectionType string
}
