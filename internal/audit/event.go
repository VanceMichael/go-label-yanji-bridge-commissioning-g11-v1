package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

type Detail map[string]any

func Event(id string, principal domain.Principal, requestID, objectType, objectID, action, result string, detail Detail, now time.Time) (domain.AuditEvent, error) {
	for field, value := range map[string]string{
		"id": id, "actor": principal.UserID, "organization": principal.Organization,
		"request_id": requestID, "object_type": objectType, "object_id": objectID,
		"action": action, "result": result,
	} {
		if strings.TrimSpace(value) == "" {
			return domain.AuditEvent{}, fmt.Errorf("audit %s: %w", field, domain.ErrInvalid)
		}
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("encode audit detail: %w", err)
	}
	return domain.AuditEvent{
		ID: id, Organization: principal.Organization, ActorID: principal.UserID,
		RequestID: requestID, ObjectType: objectType, ObjectID: objectID,
		Action: action, Result: result, Detail: string(encoded), CreatedAt: now.UTC(),
	}, nil
}
