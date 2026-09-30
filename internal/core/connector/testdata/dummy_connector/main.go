package main

import (
	"encoding/json"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type dummyConnector struct{}

func (dummyConnector) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:          "connect-dummy",
		Name:        "Dummy Connector",
		Version:     "0.0.1",
		Type:        sdk.PluginTypeConnector,
		Description: "Test-only connector plugin",
	}
}

func (dummyConnector) CreateResource(resource sdk.ConnectorResource) (string, error) {
	return "provider-" + resource.ID, nil
}

func (dummyConnector) DestroyResource(providerHandle string) error {
	return nil
}

func (dummyConnector) Execute(providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	payload, _ := json.Marshal(map[string]string{
		"provider_handle": providerHandle,
		"kind":            string(req.Kind),
		"target":          req.Target,
	})
	return sdk.ConnectorResponse{Payload: payload}, nil
}

func (dummyConnector) OpenStream(providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	return sdk.StreamOpenResponse{
		StreamID:          "dummy-stream",
		DeliverySemantics: sdk.StreamDeliveryAtLeastOnce,
		InitialCredits:    req.InitialCredits,
	}, nil
}

func (dummyConnector) ReceiveStream(providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	return sdk.StreamReceiveResponse{}, nil
}

func (dummyConnector) AckStream(providerHandle string, req sdk.StreamAckRequest) error { return nil }

func (dummyConnector) NackStream(providerHandle string, req sdk.StreamNackRequest) error { return nil }

func (dummyConnector) GrantCredits(providerHandle string, req sdk.StreamGrantCreditsRequest) error {
	return nil
}

func (dummyConnector) CloseStream(providerHandle string, req sdk.StreamCloseRequest) error { return nil }

func (dummyConnector) Probe(providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error) {
	return sdk.ResourceStatusEvent{
		ResourceID:         providerHandle,
		ProviderPluginID:   "connect-dummy",
		Status:             sdk.ResourceStatusHealthy,
		ReasonCode:         "READY",
		Message:            "dummy connector healthy",
		ObservedAtUnixNano: 1,
	}, nil
}

func (dummyConnector) Stop() error { return nil }

func main() {
	sdk.ServeConnector(dummyConnector{})
}
