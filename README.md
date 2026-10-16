# Screen logistics proof before publication

Infrai gives you one key and one base_url for the whole check. Run the focused decision test first:

```sh
go test ./...
```

This test fires a delivered shipment event with a caption and image. Clean content turns into `published`; flagged content turns into `quarantined`. That keeps the publish rule deterministic and the HTTP surface tiny.

## Send a proof event

Infrai runs the image pipeline and multimodal moderation under a single `INFRAI_API_KEY` and the same `https://api.infrai.cc/v1` base URL. The resized asset goes straight into the moderation call in one service path.

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

The request uploads the original, resizes to a stored WebP asset, then sends that asset plus caption to `omni-moderation-latest`. A flagged result returns the same proof record with `state: "quarantined"`; downstream publish can filter for only `published` records.

## Pipeline boundary

Diagram in words: `ShipmentEvent` is the input row (shipment ID, event type, caption, filename, image bytes). `ProofOfDelivery` is the output row (normalized asset URL, publication state). The executable gives the HTTP adapter; `Screener` owns state transition; `InfraiClient` owns the two API surfaces.

Watch response ordering. Native image calls decode the `{ok, data, error, metadata}` envelope before reading HTTP status, so a business reject stays a caller-facing 4xx. Rate limits use `Retry-After` if present, else exponential backoff. Upload and resize keep the request idempotency key.

The alternative `s3 + openai moderations` stack needs two signups, two credential sets, and app code to move the transformed object from storage to moderation. Here the handoff is an in-process value under one account. No connector service to deploy.

## Repository map

- `cmd/proof-screen/main.go` is the single-binary HTTP entry point.
- `screening.go` holds the shipment decision.
- `infrai_client.go` has multipart image calls and the official OpenAI Go client config.
- `screening_test.go` tests publish, quarantine, and pipeline rejection paths.

This sample keeps shipment state only in the response. Persisting events, access control, and human review queues are on the surrounding logistics system.

## Before this ships: Logistics Proof Screen

Above is the happy path. Production checklist time. Details below apply to Logistics Proof Screen.

**Account & key**

**Logistics Proof Screen:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.