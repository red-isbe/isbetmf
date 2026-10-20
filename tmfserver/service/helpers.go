package service

import (
	_ "embed"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"log/slog"

	"github.com/google/uuid"
	"github.com/hesusruiz/isbetmf/internal/errl"
	"github.com/hesusruiz/isbetmf/internal/jsone"
	repo "github.com/hesusruiz/isbetmf/tmfserver/repository"
	"github.com/hesusruiz/isbetmf/types"
)

// requiresAuthentication checks if the user is authenticated in the request.
func (svc *Service) requiresAuthentication(req *Request) *Response {
	if !req.AuthUser.IsAuthenticated {
		return ErrorResponsef(http.StatusUnauthorized, "user not authenticated")
	}
	return nil
}

// parseRequestBodyForCreateAndReplace parses the JSON body from the CREATE request into a TMFObjectMap.
func (svc *Service) parseRequestBodyForCreateAndReplace(req *Request) (repo.TMFObjectMap, *Response) {

	// Make sure the resource is supported
	res := types.GetResourceDefinition(req.ResourceName)
	if res == nil {
		return nil, ErrorResponsef(http.StatusBadRequest, "resource type %s not supported", req.ResourceName)
	}

	incomingObjectMap, err := repo.NewTMFObjectMapFromBytes(req.ResourceName, req.Body)
	if err != nil {
		return nil, ErrorResponsef(http.StatusBadRequest, "failed to bind request body: %w", errl.Error(err))
	}
	return incomingObjectMap, nil
}

// parseRequestBodyForUpdate parses the JSON body from the UPDATE request into a TMFObjectMap.
func (svc *Service) parseRequestBodyForUpdate(req *Request) (repo.TMFObjectMap, *Response) {

	// Make sure the resource is supported
	res := types.GetResourceDefinition(req.ResourceName)
	if res == nil {
		return nil, ErrorResponsef(http.StatusBadRequest, "resource type %s not supported", req.ResourceName)
	}

	var incomingObjectMap repo.TMFObjectMap
	err := jsone.Unmarshal(req.Body, &incomingObjectMap)
	if err != nil {
		return nil, ErrorResponsef(http.StatusBadRequest, "failed to bind request body: %w", errl.Error(err))
	}

	return incomingObjectMap, nil
}

// authorizeAction calls the PDP to check if the user is authorized for the current request.
func (svc *Service) authorizeAction(req *Request, obj repo.TMFObjectMap) *Response {
	if authorized, err := svc.checkAuthorization(svc.ruleEngine, req, obj); !authorized {
		return ErrorResponsef(http.StatusForbidden,
			"user %s is not authorized, object: %s, error: %w",
			req.AuthUser.OrganizationIdentifier,
			obj,
			err,
		)
	}
	slog.Debug("caller is authorized", "action", req.Action, "resource", req.ResourceName)
	return nil
}

// applyAttributeSelection filters the fields of a TMFObjectMap based on the fieldSet.
func (svc *Service) applyAttributeSelection(obj repo.TMFObjectMap, fieldSet map[string]bool) repo.TMFObjectMap {
	if len(fieldSet) == 0 {
		return obj
	}

	filteredObjectMap := make(map[string]any, 10)
	for key := range fieldSet {
		if val, ok := obj[key]; ok {
			filteredObjectMap[key] = val
		}
	}
	return filteredObjectMap
}

// parsePaginationParams extracts limit and offset from query parameters with defaults limit=10, offset=0
func (svc *Service) parsePaginationParams(req *Request) (limit int, offset int) {
	limit = 10
	offset = 0

	if val := req.QueryParams.Get("limit"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			limit = i
		}
	}

	if val := req.QueryParams.Get("offset"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			offset = i
		}
	}
	return
}

// parseFieldsParam parses the 'fields' query parameter into a field set.
func (svc *Service) parseFieldsParam(fieldsParam string) map[string]bool {
	if fieldsParam == "" {
		return nil
	}

	// Even if the user does not specify any fields, we need to return the following fields
	fieldSet := map[string]bool{
		"id":              true,
		"href":            true,
		"lastUpdate":      true,
		"version":         true,
		"@type":           true,
		"lifecycleStatus": true,
	}

	// Add the user specified fields
	if fieldsParam != "none" {
		for f := range strings.SplitSeq(fieldsParam, ",") {
			fieldSet[strings.TrimSpace(f)] = true
		}
	}
	return fieldSet
}

// verifyObjectOnPOST handles the validation of TMF metadata fields.
// We verify that the object includes the required fields as per TM Forum specs,
// and also the ones that are mandatory in our implementation.
// Note that we are lenient on accepting objects, in the sense that we accept objects with more fields than in the
// TMF specification. This is OK, as it does not compromise consistency of the objects.
func (svc *Service) verifyObjectOnPOST(req *Request, incomingObjMap repo.TMFObjectMap) *Response {

	// Check existence of required fields as per TM Forum specs for this action (CREATE) and this type of object
	actionDefinition := types.GetActionDefinition(req.ResourceName, string(req.Action))
	if actionDefinition == nil {
		return ErrorResponsef(http.StatusBadRequest, "action %s not supported for resource %s", req.Action, req.ResourceName)
	}
	for _, requiredField := range actionDefinition.Required {
		if _, ok := incomingObjMap[requiredField]; !ok {
			return ErrorResponsef(http.StatusBadRequest, "missing required field: %s", requiredField)
		}
	}

	// Set the @type, even if the user specified it, to make sure it matches the resource name
	incomingObjMap.SetType(req.ResourceName)

	// Create an id if needed.

	// If we act as a proxy, the creation of the `id` is done by the remote server.
	if svc.proxyEnabled {
		// id can never be specified by the user
		id := incomingObjMap.ID()
		if id != "" {
			return ErrorResponsef(http.StatusBadRequest, "id can never be specified by the user")
		}
		// href can never be specified by the user
		href := incomingObjMap.Href()
		if href != "" {
			return ErrorResponsef(http.StatusBadRequest, "href can never be specified by the user")
		}
	}

	// Otherwise, we manage directly the database and have to generate the id.
	if !svc.proxyEnabled {

		var id string

		// If we have to create the id for the new object, the rule is different for Organization objects.
		// Instead of generating a random identifier, we use the unique official identifier of the organization
		if incomingObjMap.IsOrganization() {

			identificationId, err := incomingObjMap.ELSIOrganizationIdentification()
			if err != nil {
				return ErrorResponsef(http.StatusBadRequest, "organizationIdentification is required in organization object")
			}

			// The id and the organizationIdentification must always carry the same "did:elsi:" identifier
			incomingObjMap.SetELSIOrganizationIdentification(identificationId)
			id = repo.OrganizationTMFID(identificationId)

		} else {

			id = fmt.Sprintf("urn:ngsi-ld:%s:%s", ToKebabCase(req.ResourceName), uuid.NewString())

		}

		// Set both the id and href fields (required in all TMF objects)
		incomingObjMap.SetID(id)
		incomingObjMap.SetHref(id)

	}

	// Seller info is compulsory in all objects.
	// As a convenience, we set the Seller info if the user did not specify it.
	objSeller, objSellerOperator, _ := incomingObjMap.GetSellerInfo("v4")
	if objSeller == "" || objSellerOperator == "" {
		// Set default seller info from caller and server operator
		if err := incomingObjMap.SetSellerInfo(svc.ServerOperatorDid, req.AuthUser.OrganizationIdentifier, "v4"); err != nil {
			return ErrorResponsef(http.StatusInternalServerError, "failed to set default seller info: %w", err)
		}
	}

	// lastUpdate is compulsory in all objects. We do not trust the user and overwrite it with the current time.
	incomingObjMap.SetLastUpdateNow()

	// Check if the caller is trying to set the lifecycleStatus of a ProductOffering to "Launched",
	// and it can only be done by the admin.
	incomingLifecycleStatus := incomingObjMap.LifecycleStatus()
	if svc.Features.OfferingLaunchOnlyByAdmin {
		if strings.EqualFold(req.ResourceName, types.ProductOffering) && strings.EqualFold(incomingLifecycleStatus, "Launched") {
			caller := req.AuthUser
			if !repo.SameOrganizations(caller.OrganizationIdentifier, svc.ServerOperatorDid) {
				return ErrorResponsef(http.StatusForbidden, "offering launch is only allowed by admin")
			}
		}
	}

	// If the object requires a lifecycleStatus, add it if not specified by the caller
	if actionDefinition.HasField("lifecycleStatus") {
		if baseStatus, ok := LifecycleStatusMandatory[req.ResourceName]; ok {
			if incomingLifecycleStatus == "" {
				incomingObjMap.SetLifecycleStatus(baseStatus)
			}
		}
	}

	if actionDefinition.HasField("lastModified") {
		incomingObjMap.SetLastModifiedNow()
	}

	if actionDefinition.HasField("version") {
		// Set the version if the user did not specify it
		if incomingObjMap.Version() == "" {
			incomingObjMap.SetVersion("0.1")
		}
	}

	// Set schemaLocation depending on the type of object, if the user did not specify one
	if incomingObjMap.SchemaLocation() == "" {
		incomingObjMap.SetDefaultSchemaLocation(actionDefinition)
	}

	return nil
}

// REPLACE is called only by an admin, so we perform less verifications
func (svc *Service) verifyObjectOnREPLACE(req *Request, incomingObjMap repo.TMFObjectMap) *Response {

	// Check existence of required fields as per TM Forum specs for this action (CREATE) and this type of object
	actionDefinition := types.GetActionDefinition(req.ResourceName, string(req.Action))
	if actionDefinition == nil {
		return ErrorResponsef(http.StatusBadRequest, "action %s not supported for resource %s", req.Action, req.ResourceName)
	}
	for _, requiredField := range actionDefinition.Required {
		if _, ok := incomingObjMap[requiredField]; !ok {
			return ErrorResponsef(http.StatusBadRequest, "missing required field: %s", requiredField)
		}
	}

	// Set the @type, even if the user specified it, to make sure it matches the resource name
	incomingObjMap.SetType(req.ResourceName)

	// Set the id and href in the object to what the user provided in the request.
	incomingObjMap.SetID(req.ID)
	incomingObjMap.SetHref(req.ID)

	// TODO: verify that the id has the proper format for the given resource

	// Seller info is compulsory in all objects.
	// As a convenience, we set the Seller info if the user did not specify it.
	objSeller, objSellerOperator, _ := incomingObjMap.GetSellerInfo("v4")
	if objSeller == "" || objSellerOperator == "" {
		// Set default seller info from caller and server operator
		if err := incomingObjMap.SetSellerInfo(svc.ServerOperatorDid, req.AuthUser.OrganizationIdentifier, "v4"); err != nil {
			return ErrorResponsef(http.StatusInternalServerError, "failed to set default seller info: %w", err)
		}
	}

	// lastUpdate is compulsory in all objects. We do not trust the user and overwrite it with the current time.
	incomingObjMap.SetLastUpdateNow()

	// Set schemaLocation depending on the type of object, if the user did not specify one
	if incomingObjMap.SchemaLocation() == "" {
		incomingObjMap.SetDefaultSchemaLocation(actionDefinition)
	}

	return nil
}

// verifyObjectOnUpdate handles the validation and updates of metadata fields during an update operation.
func (svc *Service) verifyObjectOnUpdate(req *Request, incomingObjMap repo.TMFObjectMap) *Response {

	// Follow Postel’s Law: be liberal in what you accept, conservative in what you send.
	// We will not reject the update if the user specifies a field that is not allowed in the incoming object.
	// However, we should reject the update if the user specifies an invalid field that could cause problems
	// for the local or remote servers or clients.

	// Check the non-patchable fields are not specified in the object: id, href, lastUpdate, @type, @baseType
	for _, field := range []string{"id", "href", "lastUpdate", "@type", "@baseType"} {
		if _, ok := incomingObjMap[field]; ok {
			slog.Warn("non-patchable field", "field", field)
			// Remove the field from the incoming object
			delete(incomingObjMap, field)
		}
	}

	// Check if the caller is trying to set the lifecycleStatus of a ProductOffering to "Launched"
	if svc.Features.OfferingLaunchOnlyByAdmin {
		if strings.EqualFold(req.ResourceName, types.ProductOffering) && strings.EqualFold(incomingObjMap.LifecycleStatus(), "Launched") {
			caller := req.AuthUser
			if !repo.SameOrganizations(caller.OrganizationIdentifier, svc.ServerOperatorDid) {
				return ErrorResponsef(http.StatusForbidden, "offering launch is only allowed by admin")
			}
		}
	}

	// TODO: review this, as the user may not be able to modify this data once created
	if incomingObjMap.RequiresSellerInfo(req.ResourceName) {
		objSeller, objSellerOperator, _ := incomingObjMap.GetSellerInfo("v4")
		if objSeller == "" || objSellerOperator == "" {
			// Set default seller info from caller and server operator
			if err := incomingObjMap.SetSellerInfo(svc.ServerOperatorDid, req.AuthUser.OrganizationIdentifier, "v4"); err != nil {
				return ErrorResponsef(http.StatusInternalServerError, "failed to set default seller info: %w", err)
			}
		}
	}

	// Verify the required fields depending on the type of object
	actionDefinition := types.GetActionDefinition(req.ResourceName, string(req.Action))
	if actionDefinition == nil {
		return ErrorResponsef(http.StatusBadRequest, "action %s not supported for resource %s", req.Action, req.ResourceName)
	}
	for _, requiredField := range actionDefinition.Required {
		if _, ok := incomingObjMap[requiredField]; !ok {
			return ErrorResponsef(http.StatusBadRequest, "missing required field: %s", requiredField)
		}
	}

	if actionDefinition.HasField("lastModified") {
		incomingObjMap.SetLastModifiedNow()
	}

	// lastUpdate is compulsory in all objects. We do not trust the user and overwrite it with the current time.
	incomingObjMap.SetLastUpdateNow()

	// Set schemaLocation depending on the type of object, if the user did not specify one
	if incomingObjMap.SchemaLocation() == "" {
		incomingObjMap.SetDefaultSchemaLocation(actionDefinition)
	}

	return nil
}

// mergeRFC7396 implements JSON Merge Patch (RFC 7396) to merge a patch object into a target object.
func (svc *Service) mergeRFC7396(target, patch map[string]any) {
	for k, v := range patch {
		if v == nil {
			delete(target, k)
			continue
		}

		vMap, vIsMap := v.(map[string]any)
		if vIsMap {
			if existingChild, ok := target[k]; ok {
				if existingChildMap, ok2 := existingChild.(map[string]any); ok2 {
					svc.mergeRFC7396(existingChildMap, vMap)
					target[k] = existingChildMap
					continue
				}
			}
			target[k] = vMap
			continue
		}
		target[k] = v
	}
}
