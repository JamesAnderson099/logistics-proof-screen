package proofscreen

import (
	"context"
	"errors"
	"testing"
)

type pipelineStub struct {
	flagged bool
	err     error
	calls   []string
}

func (p *pipelineStub) Upload(_ context.Context, _ ShipmentEvent, _ string) (string, error) {
	p.calls = append(p.calls, "upload")
	return "https://media.example/raw", p.err
}

func (p *pipelineStub) Resize(_ context.Context, _ string, _ string) (string, error) {
	p.calls = append(p.calls, "resize")
	return "https://media.example/ready.webp", p.err
}

func (p *pipelineStub) Flagged(_ context.Context, _ string, _ string) (bool, error) {
	p.calls = append(p.calls, "moderate")
	return p.flagged, p.err
}

func TestScreenDecision(t *testing.T) {
	tests := []struct {
		name      string
		flagged   bool
		pipeline  error
		wantState string
		wantErr   bool
	}{
		{name: "clean delivery proof is published", wantState: "published"},
		{name: "flagged delivery proof is quarantined", flagged: true, wantState: "quarantined"},
		{name: "pipeline rejection records no decision", pipeline: errors.New("rejected"), wantErr: true},
	}

	event := ShipmentEvent{ShipmentID: "shp_2048", EventType: "delivered", Caption: "Left at reception", Filename: "proof.jpg", Image: []byte("jpeg")}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := &pipelineStub{flagged: tt.flagged, err: tt.pipeline}
			got, err := NewScreener(media).Screen(context.Background(), event, "req-17")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Screen() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.State != tt.wantState {
				t.Fatalf("Screen() state = %q, want %q", got.State, tt.wantState)
			}
			if !tt.wantErr && len(media.calls) != 3 {
				t.Fatalf("pipeline calls = %v", media.calls)
			}
		})
	}
}
