package capture

type SVGIdentity struct {
	ProducerID      string `json:"producerId"`
	ProducerVersion string `json:"producerVersion"`
	PanelID         int    `json:"panelId"`
	InstanceID      string `json:"instanceId"`
}

type SVGRun struct {
	Generation      int64 `json:"generation"`
	EffectiveFromMs int64 `json:"effectiveFromMs"`
	EffectiveToMs   int64 `json:"effectiveToMs"`
}
