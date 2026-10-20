package service

import (
	"context"
	"net/url"
	"testing"

	"github.com/hesusruiz/isbetmf/config"
	repo "github.com/hesusruiz/isbetmf/tmfserver/repository"
)

// mapStorer is an in-memory TMFStorer keyed by object id, enough to exercise the id handling of the service.
type mapStorer map[string]*repo.TMFRecord

func (m mapStorer) CreateObject(obj *repo.TMFRecord) error { m[obj.ID] = obj; return nil }
func (m mapStorer) GetObject(id, resourceName string) (*repo.TMFRecord, error) {
	return m[id], nil
}
func (m mapStorer) UpdateObject(obj *repo.TMFRecord) error { m[obj.ID] = obj; return nil }
func (m mapStorer) UpsertObject(obj *repo.TMFRecord) error { m[obj.ID] = obj; return nil }
func (m mapStorer) DeleteObject(id, resourceName string) error {
	delete(m, id)
	return nil
}
func (m mapStorer) ListObjects(healthRequest bool, resourceName string, queryParams url.Values, filter repo.ObjectFilter) ([]repo.TMFRecord, error) {
	return nil, nil
}

// TestGetOrganizationWithLegacyID checks that, in every ISBE environment, an organization stored with the
// canonical id can be read with the legacy id "urn:ngsi-ld:organization:<identifier>" (ISBECORE-421).
func TestGetOrganizationWithLegacyID(t *testing.T) {
	const canonicalID = "urn:ngsi-ld:organization:did:elsi:VATES-B12345674"

	tests := []struct {
		env       config.Environment
		requestID string
		wantFound bool
	}{
		{config.ISBE_DEV, "urn:ngsi-ld:organization:VATES-B12345674", true},
		{config.ISBE_PRE, "urn:ngsi-ld:organization:VATES-B12345674", true},
		{config.ISBE_PRO, "urn:ngsi-ld:organization:VATES-B12345674", true},
		{config.ISBE_PRO, canonicalID, true},
		{config.DOME_PRO, "urn:ngsi-ld:organization:VATES-B12345674", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.env)+" "+tt.requestID, func(t *testing.T) {
			svc := &Service{
				environment: tt.env,
				storage:     mapStorer{canonicalID: {ID: canonicalID, Type: "organization"}},
			}
			req := &Request{ResourceName: "organization", ID: tt.requestID}

			obj, err := svc.getLocalOrRemoteObject(context.Background(), req)
			if err != nil {
				t.Fatalf("getLocalOrRemoteObject() error = %v", err)
			}
			if found := obj != nil; found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}
