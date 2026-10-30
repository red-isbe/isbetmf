package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestELSIOrganizationIdentification(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				map[string]any{
					"identificationType": "other",
					"identificationId":   "123",
				},
				map[string]any{
					"identificationType": elsiIdentificationType,
					"identificationId":   "did:elsi:VATES-12345678",
					"issuingAuthority":   eIDASAuthority,
				},
			},
		}

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:VATES-12345678", id)
	})

	t.Run("no organizationIdentification array", func(t *testing.T) {
		obj := TMFObjectMap{}

		id, err := obj.ELSIOrganizationIdentification()
		require.Error(t, err)
		assert.Empty(t, id)
		assert.Contains(t, err.Error(), "no organizationIdentification")
	})

	t.Run("empty organizationIdentification array", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{},
		}

		id, err := obj.ELSIOrganizationIdentification()
		require.Error(t, err)
		assert.Empty(t, id)
		assert.Contains(t, err.Error(), "no organizationIdentification")
	})

	t.Run("no elsi entry", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				map[string]any{
					"identificationType": "other",
					"identificationId":   "123",
				},
			},
		}

		id, err := obj.ELSIOrganizationIdentification()
		require.Error(t, err)
		assert.Empty(t, id)
		assert.Contains(t, err.Error(), "no identificationType elsiIdentificationType")
	})

	t.Run("skip invalid map and zero length id", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				"not a map",
				map[string]any{}, // empty map
				map[string]any{
					"identificationType": elsiIdentificationType,
					"identificationId":   "", // zero length string
				},
				map[string]any{
					"identificationType": elsiIdentificationType,
					"identificationId":   "did:elsi:VATES-12345678",
				},
			},
		}

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:VATES-12345678", id)
	})
}

func TestSetELSIOrganizationIdentification(t *testing.T) {
	t.Run("create new array and entry", func(t *testing.T) {
		obj := TMFObjectMap{}

		obj.SetELSIOrganizationIdentification("VATES-12345678")

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:VATES-12345678", id)

		orgIds := obj.GetArrayField("organizationIdentification")
		require.Len(t, orgIds, 1)

		entry := orgIds[0].(map[string]any)
		assert.Equal(t, "organizationIdentification", entry["@type"])
		assert.Equal(t, "did:elsi:VATES-12345678", entry["identificationId"])
		assert.Equal(t, elsiIdentificationType, entry["identificationType"])
		assert.Equal(t, eIDASAuthority, entry["issuingAuthority"])
	})

	t.Run("append to existing array", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				map[string]any{
					"identificationType": "other",
					"identificationId":   "123",
				},
			},
		}

		obj.SetELSIOrganizationIdentification("did:elsi:VATES-12345678")

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:VATES-12345678", id)

		orgIds := obj.GetArrayField("organizationIdentification")
		require.Len(t, orgIds, 2)
	})

	t.Run("update existing elsi entry", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				map[string]any{
					"identificationType": elsiIdentificationType,
					"identificationId":   "did:elsi:OLD-ID",
				},
			},
		}

		obj.SetELSIOrganizationIdentification("NEW-ID")

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:NEW-ID", id)

		orgIds := obj.GetArrayField("organizationIdentification")
		require.Len(t, orgIds, 1)
		entry := orgIds[0].(map[string]any)
		assert.Equal(t, "did:elsi:NEW-ID", entry["identificationId"])
	})

	t.Run("skip invalid map during update", func(t *testing.T) {
		obj := TMFObjectMap{
			"organizationIdentification": []any{
				map[string]any{}, // empty map, should be skipped
				map[string]any{
					"identificationType": elsiIdentificationType,
					"identificationId":   "did:elsi:OLD-ID",
				},
			},
		}

		obj.SetELSIOrganizationIdentification("NEW-ID")

		id, err := obj.ELSIOrganizationIdentification()
		require.NoError(t, err)
		assert.Equal(t, "did:elsi:NEW-ID", id)
	})
}

func TestOrganizationTMFID(t *testing.T) {
	const want = "urn:ngsi-ld:organization:did:elsi:VATES-B12345674"

	tests := []struct {
		name       string
		identifier string
	}{
		{"bare identifier", "VATES-B12345674"},
		{"did", "did:elsi:VATES-B12345674"},
		{"legacy id", "urn:ngsi-ld:organization:VATES-B12345674"},
		{"canonical id", want},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, want, OrganizationTMFID(tt.identifier))
		})
	}

	assert.Equal(t, "urn:ngsi-ld:organization:did:elsi:NTRES-12345678", OrganizationTMFID("NTRES-12345678"))
}
