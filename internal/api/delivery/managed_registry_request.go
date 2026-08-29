package delivery

// ManagedOCIRegistryRequest is the HTTP payload for creating or updating the
// platform-managed OCI Registry. Domain validation is delegated to service.
type ManagedOCIRegistryRequest struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Endpoint            string `json:"endpoint"`
	VerificationImage   string `json:"verification_image"`
	RegistryImage       string `json:"registry_image"`
	DataNode            string `json:"data_node"`
	PVCName             string `json:"pvc_name"`
	CPURequest          string `json:"cpu_request"`
	CPULimit            string `json:"cpu_limit"`
	MemoryRequest       string `json:"memory_request"`
	MemoryLimit         string `json:"memory_limit"`
	InsecureHTTP        bool   `json:"insecure_http"`
	ConfirmInsecureHTTP bool   `json:"confirm_insecure_http"`
	CertificateName     string `json:"certificate_name"`
	PullUsername        string `json:"pull_username"`
	PullPassword        string `json:"pull_password"`
	ProjectIDs          []uint `json:"project_ids"`
}

type ManagedOCIRegistryApplyRequest struct {
	ServerIDs []uint `json:"server_ids"`
}
