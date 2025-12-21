terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.oidc_project_id
}

########################
## Input variables
########################

variable "cluster_project_id" {
  description = "GCP project ID that owns the GKE cluster."
  type        = string
}

variable "cluster_region" {
  description = "Region of the GKE cluster (e.g. us-central1)."
  type        = string
}

variable "cluster_name" {
  description = "Name of the GKE cluster."
  type        = string
}

variable "oidc_project_id" {
  description = "GCP project ID that will own the Workload Identity Pool / OIDC provider."
  type        = string
}

variable "ksa_namespace" {
  description = "Kubernetes namespace for the tenant ServiceAccount that gsm-operator impersonates."
  type        = string
  default     = "gsmsecret-test-ns"
}

variable "ksa_name" {
  description = "Name of the Kubernetes ServiceAccount that gsm-operator impersonates."
  type        = string
  default     = "default"
}

########################
## Data sources
########################

data "google_project" "cluster" {
  project_id = var.cluster_project_id
}

data "google_project" "oidc" {
  project_id = var.oidc_project_id
}

locals {
  cluster_oidc_url = "https://container.googleapis.com/v1/projects/${var.cluster_project_id}/locations/${var.cluster_region}/clusters/${var.cluster_name}"
}

########################
## Workload Identity Pool & Provider
########################

resource "google_iam_workload_identity_pool" "gsm_operator_pool" {
  project  = var.oidc_project_id
  location = "global"

  workload_identity_pool_id = "gsm-operator-pool"
  display_name              = "GSM Operator Pool"
}

resource "google_iam_workload_identity_pool_provider" "gsm_operator_provider" {
  project  = var.oidc_project_id
  location = "global"

  workload_identity_pool_id          = google_iam_workload_identity_pool.gsm_operator_pool.workload_identity_pool_id
  workload_identity_pool_provider_id = "gsm-operator-provider"

  oidc {
    issuer_uri = local.cluster_oidc_url
  }

  attribute_mapping = {
    "google.subject" = "assertion.sub"
  }
}

########################
## Outputs (wifAudience & principals)
########################

output "wif_audience" {
  description = "Workload Identity Federation audience to place in GSMSecret.spec.wifAudience."
  value       = "//iam.googleapis.com/projects/${data.google_project.oidc.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.gsm_operator_pool.workload_identity_pool_id}/providers/${google_iam_workload_identity_pool_provider.gsm_operator_provider.workload_identity_pool_provider_id}"
}

output "ksa_principal" {
  description = "Principal string to use when granting Secret Manager access to the tenant Kubernetes ServiceAccount."
  value       = "principal://iam.googleapis.com/projects/${data.google_project.oidc.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.gsm_operator_pool.workload_identity_pool_id}/subject/system:serviceaccount:${var.ksa_namespace}:${var.ksa_name}"
}


