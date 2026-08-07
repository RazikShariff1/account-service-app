// Package notify triggers push notifications through communication-svc as a
// side effect of other packages' own operations (e.g. an account or
// individual being created), without making those operations depend on
// communication-svc being up.
package notify

import (
	"encoding/json"

	"gofr.dev/pkg/gofr"

	"main/middleware"
)

type pushRequest struct {
	MId          int            `json:"m_id"`
	TemplateName string         `json:"template_name"`
	TemplateData map[string]any `json:"template_data"`
}

// ToMasjid sends a push notification, via communication-svc, to every
// account under mID. It's best-effort: any failure is logged and swallowed
// rather than returned, so a communication-svc outage can never fail the
// caller's own request.
func ToMasjid(c *gofr.Context, mID int, templateName string, templateData map[string]any) {
	body, err := json.Marshal(pushRequest{MId: mID, TemplateName: templateName, TemplateData: templateData})
	if err != nil {
		c.Logger.Errorf("notify: failed to marshal push request for template %q: %v", templateName, err)
		return
	}

	headers := map[string]string{"Authorization": middleware.AuthHeaderFromContext(c)}

	resp, err := c.GetHTTPService("communication-svc").PostWithHeaders(c, "push/send", nil, body, headers)
	if err != nil {
		c.Logger.Errorf("notify: communication-svc push/send failed for template %q: %v", templateName, err)
		return
	}

	defer resp.Body.Close()
}
