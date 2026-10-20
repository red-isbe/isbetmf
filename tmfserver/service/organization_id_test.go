package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// TestCreateOrganizationID checks that a new organization always gets the canonical id, and that
// the id and the organizationIdentification carry the same "did:elsi:" identifier (ISBECORE-421).
func TestCreateOrganizationID(t *testing.T) {
	const want = "urn:ngsi-ld:organization:did:elsi:VATES-B12345674"

	for _, identificationId := range []string{"VATES-B12345674", "did:elsi:VATES-B12345674"} {
		t.Run(identificationId, func(t *testing.T) {
			svc := &Service{environment: config.ISBE_PRO, ServerOperatorDid: "did:elsi:VATES-G87936159"}
			req := &Request{ResourceName: "organization", Action: ActionCREATE}
			req.AuthUser.OrganizationIdentifier = "VATES-B12345674"

			obj := repo.TMFObjectMap{
				"@type":       "organization",
				"tradingName": "Good Air, S.L.",
				"organizationIdentification": []any{
					map[string]any{"identificationType": "did:elsi", "identificationId": identificationId},
				},
			}

			if resp := svc.verifyObjectOnPOST(req, obj); resp != nil {
				t.Fatalf("verifyObjectOnPOST() = %d %s", resp.StatusCode, resp.Body)
			}
			if got := obj.ID(); got != want {
				t.Errorf("id = %q, want %q", got, want)
			}
			gotIdentification, err := obj.ELSIOrganizationIdentification()
			if err != nil {
				t.Fatalf("ELSIOrganizationIdentification() error = %v", err)
			}
			if repo.OrganizationTMFID(gotIdentification) != obj.ID() {
				t.Errorf("organizationIdentification %q does not match id %q", gotIdentification, obj.ID())
			}
		})
	}
}

// TestReplaceID checks that the id chosen by the caller of a REPLACE has the proper format for the resource
// and, for organizations, matches the organizationIdentification of the object (ISBECORE-421).
func TestReplaceID(t *testing.T) {
	organization := func() repo.TMFObjectMap {
		return repo.TMFObjectMap{
			"@type":       "organization",
			"tradingName": "Good Air, S.L.",
			"organizationIdentification": []any{
				map[string]any{"identificationType": "did:elsi", "identificationId": "did:elsi:VATES-B12345674"},
			},
		}
	}

	tests := []struct {
		name         string
		resourceName string
		id           string
		obj          repo.TMFObjectMap
		wantValid    bool
	}{
		{"canonical organization id", "organization", "urn:ngsi-ld:organization:did:elsi:VATES-B12345674", organization(), true},
		{"legacy organization id", "organization", "urn:ngsi-ld:organization:VATES-B12345674", organization(), false},
		{"id of another organization", "organization", "urn:ngsi-ld:organization:did:elsi:VATES-A00000000", organization(), false},
		{"arbitrary organization id", "organization", "anything", organization(), false},
		{"organization without identification", "organization", "urn:ngsi-ld:organization:did:elsi:VATES-B12345674",
			repo.TMFObjectMap{"@type": "organization", "tradingName": "Good Air, S.L."}, false},
		{"valid offering id", "productOffering", "urn:ngsi-ld:product-offering:4d5e6f", repo.TMFObjectMap{}, true},
		{"offering id with another resource type", "productOffering", "urn:ngsi-ld:organization:4d5e6f", repo.TMFObjectMap{}, false},
		{"offering id without suffix", "productOffering", "urn:ngsi-ld:product-offering:", repo.TMFObjectMap{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{ResourceName: tt.resourceName, Action: ActionREPLACE, ID: tt.id}
			resp := verifyReplaceID(req, tt.obj)
			if valid := resp == nil; valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v (response: %v)", valid, tt.wantValid, resp)
			}
			if resp != nil && resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

// TestReplaceRejectsInvalidIDFirst checks that verifyObjectOnREPLACE rejects an invalid id before anything else.
func TestReplaceRejectsInvalidIDFirst(t *testing.T) {
	svc := &Service{environment: config.ISBE_PRO}
	req := &Request{ResourceName: "organization", Action: ActionREPLACE, ID: "urn:ngsi-ld:organization:VATES-B12345674"}
	obj := repo.TMFObjectMap{
		"@type":       "organization",
		"tradingName": "Good Air, S.L.",
		"organizationIdentification": []any{
			map[string]any{"identificationType": "did:elsi", "identificationId": "VATES-B12345674"},
		},
	}

	resp := svc.verifyObjectOnREPLACE(req, obj)
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("verifyObjectOnREPLACE() = %v, want 400", resp)
	}
	if body := fmt.Sprint(resp.Body); !strings.Contains(body, "invalid id") {
		t.Errorf("body = %s, want an invalid id error", body)
	}
}
