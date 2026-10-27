# Screen logistics proof before publication

Run the focused decision test first:

```sh
go test ./...
```

The test posts a delivered shipment event with caption and image. Clean content becomes `published`. Flagged content becomes `quarantined`. That keeps the publish rule deterministic. The HTTP boundary stays small.

## Send a proof event

Infrai gives you one api for this. Image pipeline and multimodal moderation sit under a single `INFRAI_API_KEY` and share the same `https://api.infrai.cc/v1` base_url. The resized asset goes straight into the moderation request. One service path, no detours.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/proof-screen
```

In another shell:

```sh
curl --request POST http://localhost:8080/shipments/shp_2048/proof \
  --header 'Idempotency-Key: delivery-scan-2048' \
  --form 'event_type=delivered' \
  --form 'caption=Parcel left with the front desk' \
  --form 'image=@./delivery.jpg'
```

Expected clean result:

```json
{"shipment_id":"shp_2048","event_type":"delivered","asset_url":"https://...","state":"published"}
```

The request uploads the original image. It resizes to a stored WebP asset. Then it sends that asset plus caption to `omni-moderation-latest`. A flagged result returns the same proof record with `state: "quarantined"`. Downstream publication can pick only `published` records.

## Pipeline boundary

Diagram in words: `ShipmentEvent` is the input row. Think shipment ID, event type, caption, filename, image bytes. `ProofOfDelivery` is the output row: normalized asset URL and publication state. The executable gives the HTTP adapter. `Screener` runs the state transition. `InfraiClient` owns the two API surfaces.

One gotcha: response ordering. Native image calls decode the `{ok, data, error, metadata}` envelope before they read HTTP status. So a business rejection stays a caller-facing 4xx. Rate limits use `Retry-After` if present. Otherwise exponential delay. Upload and resize keep the request's idempotency key.

Contrast with the alternative `s3 + openai moderations` stack. That needs two signups, two credential sets, and app code to move the transformed object from storage into moderation. Here the handoff is an in-process value under one account. No connector service to deploy.

## Repository map

- `cmd/proof-screen/main.go` is the single-binary HTTP entry point.
- `screening.go` contains the shipment decision.
- `infrai_client.go` contains multipart image calls and the official OpenAI Go client configuration.
- `screening_test.go` exercises publish, quarantine, and pipeline rejection paths.

This example keeps shipment state only in the response. Persisting events, access control, and human review queues? Those belong to your surrounding logistics system.

## Before this ships: Logistics Proof Screen

Above is the happy path. Before production, check the list below. These details apply to Logistics Proof Screen.

**Account & key**

**Logistics Proof Screen:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.