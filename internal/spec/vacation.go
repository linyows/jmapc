package spec

// registerVacation adds the vacation response of RFC 8621, Section 8. The
// account holds exactly one, whose id is always "singleton".
func registerVacation(s *Spec) {
	s.AddObject(&Object{
		Name:       "VacationResponse",
		Capability: CapabilityVacation,
		Doc:        "VacationResponse is the automatic reply the server sends on the user's behalf while they are away. An account has exactly one, whose id is always \"singleton\".",
		Fields: []*Field{
			{
				Name:      "id",
				Type:      "Id",
				ServerSet: true,
				Immutable: true,
				Doc:       "The id of the vacation response, which is always \"singleton\".",
			},
			{Name: "isEnabled", Type: "Boolean", Doc: "Whether the server is sending the response."},
			{
				Name: "fromDate",
				Type: "UTCDate|null",
				Doc:  "When to start sending the response, or null to start as soon as it is enabled.",
			},
			{
				Name: "toDate",
				Type: "UTCDate|null",
				Doc:  "When to stop sending the response, or null to keep sending it until it is disabled.",
			},
			{Name: "subject", Type: "String|null", Doc: "The Subject header field of the response, or null to let the server choose one."},
			{Name: "textBody", Type: "String|null", Doc: "The plain-text body of the response."},
			{Name: "htmlBody", Type: "String|null", Doc: "The HTML body of the response."},
		},
	})
	s.RegisterStandard("VacationResponse", CapabilityVacation, StandardMethods{Get: true, Set: true})
}
