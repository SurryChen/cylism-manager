package registry

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type proxyRepositoryFake struct {
	proxies []model.RegistryProxy
	saved   *model.RegistryProxy
}

func (f *proxyRepositoryFake) GetRegistryProxy() (*model.RegistryProxy, error) {
	if len(f.proxies) == 0 {
		return nil, errors.New("record not found")
	}
	return &f.proxies[0], nil
}
func (f *proxyRepositoryFake) GetRegistryProxyByID(id uint) (*model.RegistryProxy, error) {
	for i := range f.proxies {
		if f.proxies[i].ID == id {
			return &f.proxies[i], nil
		}
	}
	return nil, errors.New("record not found")
}
func (f *proxyRepositoryFake) ListRegistryProxies() ([]model.RegistryProxy, error) {
	return append([]model.RegistryProxy(nil), f.proxies...), nil
}
func (f *proxyRepositoryFake) SaveRegistryProxy(proxy *model.RegistryProxy) error {
	proxy.ID = 1
	copy := *proxy
	f.saved = &copy
	return nil
}

func TestProxyServicePersistsThroughRepositoryContract(t *testing.T) {
	repository := &proxyRepositoryFake{}
	service := NewProxyService(repository, []byte("01234567890123456789012345678901"))
	proxy, err := service.PrepareDeployment(ProxyInput{Name: "Docker Hub", Registry: "docker.io", NodeName: "node-a", EndpointHost: "100.64.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24, HTTPProxy: "http://user:secret@proxy.internal:3128"}, nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saved == nil || repository.saved.EncryptedHTTPProxy == "" || repository.saved.CreatedBy != 7 || proxy.ID != 1 {
		t.Fatalf("repository persistence mismatch: saved=%#v proxy=%#v", repository.saved, proxy)
	}
}
