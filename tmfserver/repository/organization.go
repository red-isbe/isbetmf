package repository

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/hesusruiz/isbetmf/internal/errl"
	"github.com/hesusruiz/isbetmf/internal/jpath"
	"github.com/hesusruiz/isbetmf/types"
)

const eIDASAuthority = "eIDAS"
const elsiIdentificationType = "did:elsi"

// ELSIOrganizationIdentification returns the ELSI organization identification from the object
// It returns the identificationId if found, otherwise an error
func (obj TMFObjectMap) ELSIOrganizationIdentification() (string, error) {
	// Organization identification information is located in the organizationIdentification array
	// If the object is not an Organization, or we do not find the array, we just return the empty string
	organizationIdentificationArray := jpath.GetList(obj, "organizationIdentification")
	if len(organizationIdentificationArray) == 0 {
		return "", errl.Errorf("no organizationIdentification")
	}

	// Loop the array until we find the entry with the identificationType elsiIdentificationType
	for i, identification := range organizationIdentificationArray {

		identificationMap, _ := identification.(map[string]any)

		// Invalid maps are skipped just logging a warning
		if len(identificationMap) == 0 {
			slog.Warn("invalid organizationIdentification entry", "obj", obj.String())
			continue
		}

		identificationType, _ := identificationMap["identificationType"].(string)
		if identificationType != elsiIdentificationType {
			continue
		}

		identificationId, _ := identificationMap["identificationId"].(string)
		if identificationId == "" {
			// This is an error, but we continue the loop to see if there is another entry
			slog.Warn("zero length identificationId in organizationIdentification entry", "obj", obj.String())
			continue
		}

		// Make sure that the identificationId has the prefix "did:elsi:"
		if !strings.HasPrefix(identificationId, "did:elsi:") {
			identificationId = "did:elsi:" + identificationId
			identificationMap["identificationId"] = identificationId
			organizationIdentificationArray[i] = identificationMap
		}

		return identificationId, nil
	}
	return "", errl.Errorf("no identificationType elsiIdentificationType")
}

// SetELSIOrganizationIdentification sets the ELSI organization identification in the object
// It modifies an existing entry, or creates a new one if needed
func (obj TMFObjectMap) SetELSIOrganizationIdentification(identificationId string) {

	// Normalize the identificationId to have always a prefix "did:elsi:"
	if !strings.HasPrefix(identificationId, "did:elsi:") {
		identificationId = "did:elsi:" + identificationId
	}

	// Pre-create the entry in the array
	theIdentification := map[string]any{
		"@type":              "organizationIdentification",
		"identificationId":   identificationId,
		"identificationType": elsiIdentificationType,
		"issuingAuthority":   eIDASAuthority,
	}

	// If there is no array, create it and return
	organizationIdentificationArray := jpath.GetList(obj, "organizationIdentification")
	if len(organizationIdentificationArray) == 0 {
		obj["organizationIdentification"] = []any{
			theIdentification,
		}
		return
	}

	// Loop the array until we find the entry with the identificationType elsiIdentificationType
	for _, identification := range organizationIdentificationArray {

		identificationMap, _ := identification.(map[string]any)

		// Invalid maps are skipped just logging a warning
		if len(identificationMap) == 0 {
			slog.Warn("invalid organizationIdentification entry", "obj", obj.String())
			continue
		}

		identificationType, _ := identificationMap["identificationType"].(string)
		if identificationType != elsiIdentificationType {
			continue
		}

		// Update the identificationId in the map that is part of the array
		identificationMap["identificationId"] = identificationId
		identificationMap["identificationType"] = elsiIdentificationType
		identificationMap["issuingAuthority"] = eIDASAuthority
		identificationMap["@type"] = "organizationIdentification"

		return
	}

	// No existing entry was found, add a new one
	organizationIdentificationArray = append(organizationIdentificationArray, theIdentification)
	obj["organizationIdentification"] = organizationIdentificationArray
	return

}

type Organization struct {
	CommonName             string `json:"commonName"`
	Country                string `json:"country"`
	EmailAddress           string `json:"emailAddress"`
	Organization           string `json:"organization"`
	OrganizationIdentifier string `json:"organizationIdentifier"`
	SerialNumber           string `json:"serialNumber"`
}

func (u *Organization) ToMap() map[string]any {
	return map[string]any{
		"commonName":             u.CommonName,
		"country":                u.Country,
		"emailAddress":           u.EmailAddress,
		"organization":           u.Organization,
		"organizationIdentifier": u.OrganizationIdentifier,
		"serialNumber":           u.SerialNumber,
	}
}

func TMFRecordFromOrganizationAndToken(user *Organization, accessToken map[string]any) (*TMFRecord, error) {

	// Generate the DID for the organization and the TMF object id
	did := user.OrganizationIdentifier
	if !strings.HasPrefix(did, "did:elsi:") {
		did = "did:elsi:" + did
	}
	id := "urn:ngsi-ld:organization:" + did

	now := time.Now()
	lastUpdate := now.Format(time.RFC3339Nano)

	objectType := types.Organization
	version := "1.0"

	theIdentification := map[string]any{
		"@type":              "organizationIdentification",
		"identificationId":   did,
		"identificationType": elsiIdentificationType,
		"issuingAuthority":   eIDASAuthority,
	}

	if accessToken != nil {
		tokenJSON, err := json.Marshal(accessToken)
		if err != nil {
			return nil, errl.Errorf("error marshalling access token: %w", err)
		}

		// Attach the access token that justifies the creation of the object
		attch := map[string]any{
			"@type":       "attachment",
			"name":        "verifiableCredential",
			"contentType": "application/json",
			"content":     base64.StdEncoding.EncodeToString(tokenJSON),
		}

		theIdentification["attachment"] = attch

	}

	// Prepare organizationIdentification
	orgIdentification := []any{
		theIdentification,
	}

	// Prepare contactMedium
	var contactMedium []any
	if user.EmailAddress != "" {
		contactMedium = append(contactMedium, map[string]any{
			"@type":        "EmailContactMedium",
			"preferred":    true,
			"emailAddress": user.EmailAddress,
		})
	}

	orgMap := map[string]any{
		"@type":                      objectType,
		"isLegalEntity":              true,
		"id":                         id,
		"href":                       id,
		"version":                    version,
		"lastUpdate":                 lastUpdate,
		"name":                       user.Organization,
		"tradingName":                user.Organization,
		"contactMedium":              contactMedium,
		"organizationIdentification": orgIdentification,
		"externalReference": []any{
			map[string]any{
				"externalReferenceType": "idm_id",
				"name":                  user.OrganizationIdentifier,
			},
		},
		"partyCharacteristic": []any{
			map[string]any{
				"name":  "country",
				"value": user.Country,
			},
		},
	}

	content, err := json.Marshal(orgMap)
	if err != nil {
		return nil, errl.Errorf("error marshalling organization: %w", err)
	}

	org := &TMFRecord{
		ID:         id,
		Type:       objectType,
		Version:    version,
		APIVersion: "v4",
		LastUpdate: lastUpdate,
		Content:    content,
		CreatedAt:  now.Unix(),
		UpdatedAt:  now.Unix(),
	}

	return org, nil
}

func SameOrganizations(did, orgID string) bool {

	did = strings.TrimPrefix(did, "did:elsi:")
	orgID = strings.TrimPrefix(orgID, "did:elsi:")

	return did == orgID
}

// OrganizationTMFID returns the canonical TMF id of an organization, "urn:ngsi-ld:organization:did:elsi:<identifier>".
// It accepts the organization identifier in any of its forms: "VATES-X", "did:elsi:VATES-X",
// "urn:ngsi-ld:organization:VATES-X" or "urn:ngsi-ld:organization:did:elsi:VATES-X".
func OrganizationTMFID(identifier string) string {
	identifier = strings.TrimPrefix(identifier, "urn:ngsi-ld:organization:")
	identifier = strings.TrimPrefix(identifier, "did:elsi:")
	return "urn:ngsi-ld:organization:did:elsi:" + identifier
}
