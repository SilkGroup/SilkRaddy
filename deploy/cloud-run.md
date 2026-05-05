# Cloud Run deployment runbook

Target stack: **Cloud Run** (Go server) + **Cloud SQL Postgres** (track DB) +
**Cloud Storage** (HLS segments and originals). This document is the operator
runbook; it is not consumed by any tooling.

## Status

| Phase | Scope | Status |
| --- | --- | --- |
| 0 | Cloud Run runtime compatibility (`$PORT`, graceful SIGTERM shutdown) | done |
| 1 | Postgres `Store` implementation alongside the existing sqlite one | pending |
| 2 | `FileStore` interface with `localFS` and `gcs` backends for tracks/segments | pending |
| 3 | Deploy artifacts and Terraform/`gcloud` automation | pending |

A Phase 0 deploy boots and listens on `$PORT`, but state is still on the
container's ephemeral disk. Treat it as a smoke test until Phases 1 and 2 land.

## Variables

```bash
export PROJECT_ID=silkevents-ug
export REGION=europe-west1     # closest stable region; africa-south1 is closer geographically but has narrower service parity
export REPO=silkraddy
export SERVICE=silkraddy
gcloud config set project "$PROJECT_ID"
```

## One-time setup

```bash
# Enable required APIs
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com

# Image registry
gcloud artifacts repositories create "$REPO" \
  --repository-format=docker \
  --location="$REGION"

# Application secrets (the Go config refuses to start without these)
printf '%s' "$(openssl rand -hex 32)" | gcloud secrets create AIRSTATION_JWT_SIGN   --data-file=-
printf '%s' "$(openssl rand -hex 32)" | gcloud secrets create AIRSTATION_SECRET_KEY --data-file=-

# Grant Cloud Run's default compute SA access to the secrets
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
for s in AIRSTATION_JWT_SIGN AIRSTATION_SECRET_KEY; do
  gcloud secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:$SA" \
    --role="roles/secretmanager.secretAccessor"
done
```

## Build and deploy (every release)

```bash
# Build with Cloud Build using the existing Dockerfile
gcloud builds submit \
  --tag "$REGION-docker.pkg.dev/$PROJECT_ID/$REPO/server:latest" \
  .

# Deploy to Cloud Run
gcloud run deploy "$SERVICE" \
  --image "$REGION-docker.pkg.dev/$PROJECT_ID/$REPO/server:latest" \
  --region "$REGION" \
  --allow-unauthenticated \
  --port 8080 \
  --memory 1Gi \
  --cpu 1 \
  --timeout 300 \
  --min-instances 1 \
  --max-instances 1 \
  --set-secrets AIRSTATION_JWT_SIGN=AIRSTATION_JWT_SIGN:latest,AIRSTATION_SECRET_KEY=AIRSTATION_SECRET_KEY:latest
```

Flag rationale:

- `--port 8080` — explicit; matches the `$PORT` Cloud Run injects, which the
  runtime now honours via `getEnv("AIRSTATION_HTTP_PORT", getEnv("PORT", "7331"))`.
- `--memory 1Gi` — ffmpeg processing pushes 512Mi to its limits.
- `--min-instances 1` — keeps the playback goroutine alive between listeners
  so the radio does not stop streaming when traffic drops to zero.
- `--max-instances 1` — required until Phase 1: with SQLite on ephemeral disk,
  multiple instances would each have their own database and contradict each
  other. Lift this once Postgres is wired up.

## Pending — Phases 1 and 2 setup

These commands describe what will be needed once the corresponding code lands.
Do not run them yet for production; the binary still ignores both Postgres and
GCS.

```bash
# Cloud SQL Postgres (Phase 1)
gcloud sql instances create silkraddy-db \
  --database-version=POSTGRES_16 \
  --tier=db-f1-micro \
  --region="$REGION"
gcloud sql databases create silkraddy --instance=silkraddy-db
gcloud sql users create silkraddy --instance=silkraddy-db --password="$(openssl rand -hex 24)"

# GCS bucket for tracks and HLS segments (Phase 2)
gsutil mb -l "$REGION" "gs://$PROJECT_ID-silkraddy-media"
gsutil iam ch "serviceAccount:$SA:roles/storage.objectAdmin" "gs://$PROJECT_ID-silkraddy-media"

# Wire the Postgres connection into the Cloud Run service (Phase 1)
gcloud run services update "$SERVICE" \
  --region "$REGION" \
  --add-cloudsql-instances "$PROJECT_ID:$REGION:silkraddy-db" \
  --update-env-vars AIRSTATION_DB_DRIVER=postgres
```
