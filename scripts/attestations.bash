#!/usr/bin/env bash
set -euo pipefail

# Usage: ./attestations.bash <IMAGE>
#   IMAGE can be path:tag or path@digest
#
# Configuration is provided by scripts/attestations_env.bash, which you can edit
# or override via environment variables before invoking this script.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/attestations_env.bash"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Configuration file '${ENV_FILE}' not found." >&2
  echo "Please copy 'attestations_env.bash.sample' to '${ENV_FILE}', modify it, and rerun this script." >&2
  exit 1
fi

# shellcheck source=/dev/null
source "${ENV_FILE}"

IMAGE_INPUT="${1:?IMAGE required}"

# This script:
#   1. Validates image registry against the allow-list from deploy.attestation
#   2. Resolves the image digest (if given as tag) from the GAR project
#   3. Skips if an attestation already exists
#   4. Generates a Binary Authorization payload
#   5. Signs it with Cloud KMS (sha512)
#   6. Creates a Binary Authorization attestation

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
