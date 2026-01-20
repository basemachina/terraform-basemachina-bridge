# ========================================
# プロジェクトとリージョン設定
# ========================================

variable "project_id" {
  description = "Google Cloud Project ID"
  type        = string
}

variable "region" {
  description = "Google Cloud region"
  type        = string
  default     = "asia-northeast1"
}

variable "service_name" {
  description = "Service name prefix for all resources"
  type        = string
  default     = "basemachina-bridge-example"
}

# ========================================
# Bridge環境変数
# ========================================

variable "tenant_id" {
  description = "Tenant ID for BaseMachina Bridge"
  type        = string
  sensitive   = true
}

variable "fetch_interval" {
  description = "Interval for fetching public keys"
  type        = string
  default     = "1h"
}

variable "fetch_timeout" {
  description = "Timeout for fetching public keys"
  type        = string
  default     = "10s"
}

variable "port" {
  description = "Port number for Bridge container"
  type        = number
  default     = 8080
}

# ========================================
# リソース設定
# ========================================

variable "cpu" {
  description = "CPU allocation for Cloud Run service"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory allocation for Cloud Run service"
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

variable "vpc_egress" {
  description = "VPC egress setting"
  type        = string
  default     = "PRIVATE_RANGES_ONLY"
}

# ========================================
# Load Balancerとドメイン設定
# ========================================

variable "domain_name" {
  description = "Custom domain name for the Bridge (optional)"
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
  description = "IP ranges allowed to access the service"
  type        = list(string)
  default     = ["34.85.43.93/32"]
}

# ========================================
# Cloud DNS設定
# ========================================

variable "dns_zone_name" {
  description = "Cloud DNS Managed Zone name (optional)"
  type        = string
  default     = null
}

# ========================================
# Cloud SQL設定
# ========================================

variable "database_name" {
  description = "Database name"
  type        = string
  default     = "sampledb"
}

variable "database_user" {
  description = "Database user name"
  type        = string
  default     = "dbuser"
}

# ========================================
# ラベル管理
# ========================================

variable "labels" {
  description = "Labels to apply to all resources"
  type        = map(string)
  default = {
    environment = "example"
    managed_by  = "terraform"
  }
}
