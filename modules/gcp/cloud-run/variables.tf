# ========================================
# プロジェクトとリージョン設定
# ========================================

variable "project_id" {
  description = "Google Cloud Project ID where resources will be created"
  type        = string
}

variable "region" {
  description = "Google Cloud region for Cloud Run service"
  type        = string
  default     = "asia-northeast1"
}

variable "service_name" {
  description = "Name of the Cloud Run service"
  type        = string
  default     = "basemachina-bridge"
}

# ========================================
# Bridge環境変数
# ========================================

variable "tenant_id" {
  description = "Tenant ID for authentication"
  type        = string
  sensitive   = true
}

variable "fetch_interval" {
  description = "Interval for fetching public keys (e.g., 1h, 30m)"
  type        = string
  default     = "1h"
}

variable "fetch_timeout" {
  description = "Timeout for fetching public keys (e.g., 10s, 30s)"
  type        = string
  default     = "10s"
}

variable "port" {
  description = "Container port number (cannot be 4321). Cloud Run automatically sets PORT environment variable to this value."
  type        = number
  default     = 8080

  validation {
    condition     = var.port != 4321
    error_message = "Port 4321 is not allowed"
  }
}

# ========================================
# リソース設定
# ========================================

variable "bridge_image_tag" {
  description = "Bridge container image tag (default: latest). Specify a specific version like 'v1.0.0' if needed."
  type        = string
  default     = "latest"
}

variable "cpu" {
  description = "CPU allocation for Cloud Run service (e.g., '1', '2', '4')"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory allocation for Cloud Run service (e.g., '512Mi', '1Gi', '2Gi')"
  type        = string
  default     = "512Mi"
}

variable "min_instances" {
  description = "Minimum number of instances"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum number of instances"
  type        = number
  default     = 10
}

# ========================================
# VPCネットワーク設定
# ========================================

variable "vpc_connector_id" {
  description = "VPC Access Connector ID for Cloud SQL connection (optional, uses Direct VPC Egress if not specified)"
  type        = string
  default     = null
}

variable "vpc_egress" {
  description = "VPC egress setting (ALL_TRAFFIC or PRIVATE_RANGES_ONLY)"
  type        = string
  default     = "PRIVATE_RANGES_ONLY"

  validation {
    condition     = contains(["ALL_TRAFFIC", "PRIVATE_RANGES_ONLY"], var.vpc_egress)
    error_message = "VPC egress must be either 'ALL_TRAFFIC' or 'PRIVATE_RANGES_ONLY'"
  }
}

variable "vpc_network_id" {
  description = "VPC network ID for Direct VPC Egress (optional, required if using Direct VPC Egress)"
  type        = string
  default     = null
}

variable "vpc_subnetwork_id" {
  description = "VPC subnetwork ID for Direct VPC Egress (optional, required if using Direct VPC Egress)"
  type        = string
  default     = null
}

# ========================================
# Load Balancerとドメイン設定
# ========================================

variable "domain_name" {
  description = "Custom domain name for the Bridge (optional, required for HTTPS)"
  type        = string
  default     = null
}

variable "enable_https_redirect" {
  description = "Enable HTTP to HTTPS redirect"
  type        = bool
  default     = true
}

variable "enable_cloud_armor" {
  description = "Enable Cloud Armor security policy"
  type        = bool
  default     = true
}

variable "allowed_ip_ranges" {
  description = "Additional IP ranges allowed to access the service. BaseMachina IP (34.85.43.93/32) is automatically included unless '*' is specified to allow all IPs."
  type        = list(string)
  default     = []
}

# ========================================
# Cloud DNS設定
# ========================================

variable "dns_zone_name" {
  description = "Cloud DNS Managed Zone name (optional, required for DNS record creation)"
  type        = string
  default     = null
}

# ========================================
# ラベル管理
# ========================================

variable "labels" {
  description = "Labels to apply to all resources"
  type        = map(string)
  default     = {}
}
