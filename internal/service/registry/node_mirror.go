package registry

import (
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"sigs.k8s.io/yaml"
)

// NodeMirrorApplier is the infrastructure adapter used to apply an already
// rendered K3s Registry configuration to a managed node. The Registry domain
// owns the contract; API packages provide the SSH implementation.
type NodeMirrorApplier func(*model.Server, []byte) (string, string)

// RenderK3sRegistries renders enabled Registry mirrors into K3s configuration.
// Credentials are supplied by the caller after it has decrypted them.
func RenderK3sRegistries(mirrors []model.NodeRegistryMirror, credentials map[uint]string) ([]byte, error) {
	configs, registryMirrors := map[string]interface{}{}, map[string]interface{}{}
	for _, mirror := range mirrors {
		if !mirror.Enabled {
			continue
		}
		var endpoints []string
		if err := yaml.Unmarshal([]byte(mirror.Endpoints), &endpoints); err != nil {
			return nil, fmt.Errorf("镜像源 %q 配置损坏", mirror.Name)
		}
		registryMirrors[mirror.Registry] = map[string]interface{}{"endpoint": endpoints}
		if mirror.Username != "" || mirror.Credential != "" || mirror.InsecureSkipVerify {
			config := map[string]interface{}{}
			if mirror.Username != "" {
				config["auth"] = map[string]string{"username": mirror.Username, "password": credentials[mirror.ID]}
			}
			if mirror.InsecureSkipVerify {
				config["tls"] = map[string]bool{"insecure_skip_verify": true}
			}
			configs[mirror.Registry] = config
		}
	}
	if len(registryMirrors) == 0 {
		return nil, fmt.Errorf("没有已启用的节点镜像源")
	}
	return yaml.Marshal(map[string]interface{}{"mirrors": registryMirrors, "configs": configs})
}

// RenderK3sRegistriesWithStoredCredentials resolves encrypted mirror
// credentials only while rendering the configuration that will be applied to
// a node. The returned YAML contains plaintext credentials by design and must
// not be persisted or returned through an API response.
func RenderK3sRegistriesWithStoredCredentials(mirrors []model.NodeRegistryMirror, encKey []byte) ([]byte, error) {
	credentials := make(map[uint]string)
	for _, mirror := range mirrors {
		if !mirror.Enabled || mirror.Username == "" {
			continue
		}
		if mirror.Credential == "" {
			return nil, fmt.Errorf("镜像源 %q 缺少密码或 Token", mirror.Name)
		}
		credential, err := DecryptCredential(encKey, mirror.Credential)
		if err != nil {
			return nil, fmt.Errorf("解密镜像源 %q 凭据失败", mirror.Name)
		}
		credentials[mirror.ID] = credential
	}
	return RenderK3sRegistries(mirrors, credentials)
}
