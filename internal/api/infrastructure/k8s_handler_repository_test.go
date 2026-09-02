package infrastructure

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type resourceReferenceFake struct{ references []model.ResourceReference }

func (f resourceReferenceFake) ListResourceReferences(namespace, sourceType, sourceName string) ([]model.ResourceReference, error) {
	return f.references, nil
}

func TestK8sHandlerProtectsReferencedResourceThroughRepository(t *testing.T) {
	handler := NewK8sHandlerWithAdapter(nil, resourceReferenceFake{references: []model.ResourceReference{{ApplicationID: 1, Kind: "template", Name: "default"}}}, nil, nil)
	referenced, err := handler.resourceIsReferenced("default", "secret", "api-token")
	if err != nil || !referenced {
		t.Fatalf("expected reference protection, referenced=%v err=%v", referenced, err)
	}
}
