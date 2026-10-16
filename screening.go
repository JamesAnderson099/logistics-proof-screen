package proofscreen

import (
	"context"
	"fmt"
)

type ShipmentEvent struct {
	ShipmentID string
	EventType  string
	Caption    string
	Filename   string
	Image      []byte
}

type ProofOfDelivery struct {
	ShipmentID string `json:"shipment_id"`
	EventType  string `json:"event_type"`
	AssetURL   string `json:"asset_url"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
}

type MediaPipeline interface {
	Upload(context.Context, ShipmentEvent, string) (string, error)
	Resize(context.Context, string, string) (string, error)
	Flagged(context.Context, string, string) (bool, error)
}

type Screener struct {
	media MediaPipeline
}

func NewScreener(media MediaPipeline) *Screener { return &Screener{media: media} }

func (s *Screener) Screen(ctx context.Context, event ShipmentEvent, requestID string) (ProofOfDelivery, error) {
	if event.ShipmentID == "" || event.EventType == "" || event.Caption == "" || event.Filename == "" || len(event.Image) == 0 {
		return ProofOfDelivery{}, fmt.Errorf("shipment_id, event_type, caption, filename, and image are required")
	}

	uploaded, err := s.media.Upload(ctx, event, requestID+":upload")
	if err != nil {
		return ProofOfDelivery{}, fmt.Errorf("upload proof: %w", err)
	}
	ready, err := s.media.Resize(ctx, uploaded, requestID+":resize")
	if err != nil {
		return ProofOfDelivery{}, fmt.Errorf("prepare proof: %w", err)
	}
	flagged, err := s.media.Flagged(ctx, event.Caption, ready)
	if err != nil {
		return ProofOfDelivery{}, fmt.Errorf("screen proof: %w", err)
	}

	proof := ProofOfDelivery{ShipmentID: event.ShipmentID, EventType: event.EventType, AssetURL: ready, State: "published"}
	if flagged {
		proof.State = "quarantined"
		proof.Reason = "content review required"
	}
	return proof, nil
}
