#!/usr/bin/env bash
set -euo pipefail

# Usage: ./attest.sh <IMAGE>
# IMAGE can be path:tag or path@digest, e.g.:
#   us-docker.pkg.dev/wf-gcp-us-plat-gar-prod/docker-dev-deployed/foo/bar:1.2.3
#
# This script:
#   1. Validates image registry against the allow-list from deploy.attestation
#   2. Resolves the image digest (if given as tag) from the GAR project
#   3. Skips if an attestation already exists
#   4. Generates a Binary Authorization payload
#   5. Signs it with Cloud KMS (sha512)
#   6. Creates a Binary Authorization attestation

ATTESTOR_PROJECT_ID="${ATTESTOR_PROJECT_ID:-wf-gcp-us-plat-attestor-prod}"
ATTESTOR_NAME="${ATTESTOR_NAME:-decomposed-prod-attestor}"

# GAR project is separate from the attestor project (matches GAR_PROJECT in repo)
GAR_PROJECT_ID="${GAR_PROJECT_ID:-wf-gcp-us-plat-gar-prod}"

IMAGE_INPUT="${1:?IMAGE required}"

# KMS key that holds the attestor's private key (matches PROD_* in repo)
KMS_PROJECT_ID="wf-gcp-us-plat-attestor-prod"
KMS_LOCATION="global"
KMS_KEYRING="decomposed-prod-key-ring"
KMS_KEY="decomposed-prod-attestor-key"
KMS_VERSION="1"

# Allow-listed registries (from deploy.attestation.attest_image)
ALLOWED_REGISTRIES=(
  "us-docker.pkg.dev/wf-gcp-us-plat-gar-dev/gsm-operator/gsm-operator"
#   "us-docker.pkg.dev/wf-gcp-us-plat-gar-prod/docker-prod-deployed"
#   "us-docker.pkg.dev/wf-gcp-us-plat-gar-prod/docker-dev-deployed"
#   "us-docker.pkg.dev/wf-gcp-us-plat-gar-prod/external"
#   "us-docker.pkg.dev/wf-gcp-us-plat-gar-prod/external-staging"
)

# 0) Validate registry allow-list
is_allowed="false"
for reg in "${ALLOWED_REGISTRIES[@]}"; do
  if [[ "${IMAGE_INPUT}" == "${reg}"* ]]; then
    is_allowed="true"
    break
  fi
done

if [[ "${is_allowed}" != "true" ]]; then
  echo "Image '${IMAGE_INPUT}' does not match any known base registry." >&2
  exit 1
fi

TMP_PAYLOAD="$(mktemp)"
TMP_SIGNATURE="$(mktemp)"

cleanup() {
  rm -f "${TMP_PAYLOAD}" "${TMP_SIGNATURE}"
}
trap cleanup EXIT

# 1) Resolve digest if needed (from GAR project, not attestor project)
if [[ "${IMAGE_INPUT}" == *@* ]]; then
  IMAGE_TO_ATTEST="${IMAGE_INPUT}"
else
  # IMAGE_INPUT is path:tag; split into path and tag
  IMAGE_PATH="${IMAGE_INPUT%%:*}"
  IMAGE_TAG="${IMAGE_INPUT##*:}"

  DIGEST="$(gcloud artifacts docker images describe \
    "${IMAGE_PATH}:${IMAGE_TAG}" \
    --project="${GAR_PROJECT_ID}" \
    --format='get(image_summary.digest)')"

  if [[ -z "${DIGEST}" ]]; then
    echo "Failed to resolve digest for ${IMAGE_INPUT}" >&2
    exit 1
  fi

  IMAGE_TO_ATTEST="${IMAGE_PATH}@${DIGEST}"
fi

echo "Attesting image: ${IMAGE_TO_ATTEST}"

# 2) Pre-check: skip if already attested (similar to verify_attestation)
EXISTING_ATTESTATION="$(gcloud container binauthz attestations list \
  --attestor="${ATTESTOR_NAME}" \
  --attestor-project="${ATTESTOR_PROJECT_ID}" \
  --format='value(name)' \
  --filter="resourceUri=\"${IMAGE_TO_ATTEST}\"" 2>/dev/null || true)"

if [[ -n "${EXISTING_ATTESTATION}" ]]; then
  echo "Image already attested; nothing to do."
  exit 0
fi

# 3) Create Binary Authorization payload
gcloud container binauthz create-signature-payload \
  --artifact-url="${IMAGE_TO_ATTEST}" \
  > "${TMP_PAYLOAD}"

# 4) Sign payload with Cloud KMS (sha512)
gcloud kms asymmetric-sign \
  --project="${KMS_PROJECT_ID}" \
  --location="${KMS_LOCATION}" \
  --keyring="${KMS_KEYRING}" \
  --key="${KMS_KEY}" \
  --version="${KMS_VERSION}" \
  --digest-algorithm=sha512 \
  --input-file="${TMP_PAYLOAD}" \
  --signature-file="${TMP_SIGNATURE}"

# 5) Get public key ID from attestor
PUBLIC_KEY_ID="$(gcloud container binauthz attestors describe "${ATTESTOR_NAME}" \
  --project="${ATTESTOR_PROJECT_ID}" \
  --format='value(userOwnedGrafeasNote.publicKeys[0].id)')"

if [[ -z "${PUBLIC_KEY_ID}" ]]; then
  echo "Failed to get PUBLIC_KEY_ID for attestor ${ATTESTOR_NAME}" >&2
  exit 1
fi

# 6) Create attestation
gcloud container binauthz attestations create \
  --project="${ATTESTOR_PROJECT_ID}" \
  --artifact-url="${IMAGE_TO_ATTEST}" \
  --attestor="projects/${ATTESTOR_PROJECT_ID}/attestors/${ATTESTOR_NAME}" \
  --signature-file="${TMP_SIGNATURE}" \
  --public-key-id="${PUBLIC_KEY_ID}" \
  --validate

echo "Attestation created successfully for ${IMAGE_TO_ATTEST}"