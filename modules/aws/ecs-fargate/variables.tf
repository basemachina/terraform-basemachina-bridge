# ========================================
# ネットワーク関連変数
# ========================================

variable "vpc_id" {
  description = "VPC ID where the resources will be created"
  type        = string
}

variable "private_subnet_ids" {
  description = "List of private subnet IDs for ECS tasks"
  type        = list(string)

  validation {
    condition     = length(var.private_subnet_ids) > 0
    error_message = "At least one private subnet must be specified"
  }
}

variable "public_subnet_ids" {
  description = "List of public subnet IDs for ALB and NAT Gateway (if creating new NAT Gateway)"
  type        = list(string)

  validation {
    condition     = length(var.public_subnet_ids) > 0
    error_message = "At least one public subnet must be specified"
  }
}

variable "nat_gateway_id" {
  description = "Existing NAT Gateway ID to use (optional). If not specified, a new NAT Gateway will be created for Bridge."
  type        = string
  default     = null
}

variable "create_nat_gateway" {
  description = "Whether to manage NAT Gateway resources (NAT Gateway, EIP and default routes for the private subnets). Set to false when the VPC already has NAT routing configured for the private subnets. When true with nat_gateway_id specified, the NAT Gateway itself is not created and only the default routes to the existing NAT Gateway are added."
  type        = bool
  default     = true
}

variable "create_vpc_endpoints" {
  description = "Whether to create VPC endpoints (ECR API/DKR, S3, CloudWatch Logs). Set to false when the VPC already has these endpoints, because only one interface endpoint with private DNS enabled can exist per service in a VPC."
  type        = bool
  default     = true
}

variable "enable_ecr_pull_through_cache" {
  description = "Whether to create an ECR pull through cache rule and pull the Bridge image through the private ECR registry. Set to false to pull the image directly from public.ecr.aws (requires internet access via NAT Gateway)."
  type        = bool
  default     = true
}

# ========================================
# セキュリティ関連変数
# ========================================

variable "certificate_arn" {
  description = "ACM certificate ARN for HTTPS listener (required)"
  type        = string
}

variable "ssl_policy" {
  description = "SSL security policy for the ALB HTTPS listener"
  type        = string
  default     = "ELBSecurityPolicy-TLS13-1-2-Res-PQ-2025-09"
}

variable "additional_alb_ingress_cidrs" {
  description = "Additional CIDR blocks to allow HTTPS access to ALB (for testing or additional clients). BaseMachina IP (34.85.43.93/32) is always included."
  type        = list(string)
  default     = []
}

# ========================================
# Bridge環境変数
# ========================================

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
  description = "Port number for Bridge container (cannot be 4321)"
  type        = number
  default     = 8080

  validation {
    condition     = var.port != 4321
    error_message = "Port 4321 is not allowed"
  }
}

variable "tenant_id" {
  description = "Tenant ID for authentication"
  type        = string
  sensitive   = true
}

# ========================================
# リソース設定変数
# ========================================

variable "bridge_image_tag" {
  description = "Bridge container image tag (default: latest). Specify a specific version like 'v1.0.0' if needed."
  type        = string
  default     = "latest"
}

variable "cpu" {
  description = "CPU units for ECS task (256, 512, 1024, 2048, 4096)"
  type        = number
  default     = 256

  validation {
    condition     = contains([256, 512, 1024, 2048, 4096], var.cpu)
    error_message = "CPU must be one of: 256, 512, 1024, 2048, 4096"
  }
}

variable "memory" {
  description = "Memory (MiB) for ECS task"
  type        = number
  default     = 512
}

variable "desired_count" {
  description = "Number of ECS tasks to run"
  type        = number
  default     = 1

  validation {
    condition     = var.desired_count >= 1
    error_message = "Desired count must be at least 1"
  }
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention period (days)"
  type        = number
  default     = 7
}

# ========================================
# タグ付けと命名変数
# ========================================

variable "tags" {
  description = "Common tags to apply to all resources"
  type        = map(string)
  default     = {}
}

variable "name_prefix" {
  description = "Prefix for resource names"
  type        = string
  default     = ""

  validation {
    condition     = var.name_prefix == "" || can(regex("^[0-9A-Za-z]([0-9A-Za-z-]*[0-9A-Za-z])?$", var.name_prefix))
    error_message = "name_prefix must contain only alphanumeric characters and hyphens, and must not start or end with a hyphen."
  }

  validation {
    condition     = length(var.name_prefix) <= 8
    error_message = "name_prefix must be 8 characters or fewer. This module builds names such as \"<name_prefix>-basemachina-bridge-execution-\" for the IAM role, and IAM role names are limited to 64 characters (including a 26-character suffix generated by AWS), so longer values will fail to apply."
  }
}

locals {
  app_name          = "basemachina-bridge"
  resource_prefix   = var.name_prefix != "" ? "${var.name_prefix}-" : ""
  prefixed_app_name = "${local.resource_prefix}${local.app_name}"
}

# ========================================
# Route53 / DNS関連変数
# ========================================

variable "domain_name" {
  description = "Custom domain name for the Bridge (required). This domain will be used for ALB access. An A record alias to ALB will be created automatically in the specified Route53 Hosted Zone."
  type        = string
}

variable "route53_zone_id" {
  description = "Route53 Hosted Zone ID for DNS record creation (required). An A record alias pointing to the ALB will be created automatically in this zone."
  type        = string
}
