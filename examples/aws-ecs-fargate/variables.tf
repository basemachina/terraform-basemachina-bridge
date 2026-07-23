# ========================================
# ネットワーク関連変数
# ========================================

variable "vpc_id" {
  description = "デプロイ先のVPC ID"
  type        = string
}

variable "private_subnet_ids" {
  description = "Bridgeタスクを配置するプライベートサブネットIDのリスト（複数AZ推奨）"
  type        = list(string)
}

variable "public_subnet_ids" {
  description = "ALBとNAT Gatewayを配置するパブリックサブネットIDのリスト（複数AZ推奨）"
  type        = list(string)
}

variable "nat_gateway_id" {
  description = "既存のNAT Gateway IDを使用する場合に指定（オプション）。指定しない場合は新しいNAT Gatewayが作成されます。"
  type        = string
  default     = null
}

# ========================================
# SSL/TLS証明書
# ========================================

# NOTE: 現在この変数はどこからも参照されていない（ALBの証明書は常に
# route53_domain.tf の aws_acm_certificate.bridge が使われる）。
# 外部証明書を渡せるようにする配線の修正は別PRで対応予定のため、
# 利用者向けインターフェースとして宣言のみ維持している。
# tflint-ignore: terraform_unused_declarations
variable "certificate_arn" {
  description = "HTTPS通信用のACM証明書ARN（オプション、未指定の場合はHTTPリスナーを使用）"
  type        = string
  default     = null
}

variable "enable_acm_import" {
  description = "ローカル証明書をACMにインポートする（テスト環境向け）。trueの場合、certs/ディレクトリの自己署名証明書をACMにインポートします。本番環境では使用しないでください。"
  type        = bool
  default     = false
}

variable "additional_alb_ingress_cidrs" {
  description = "ALBへのHTTPSアクセスを許可する追加のCIDRブロック（テストや追加クライアント用）。BaseMachina IP (34.85.43.93/32) は常に含まれます。"
  type        = list(string)
  default     = []
}

# ========================================
# Bridge環境変数
# ========================================

variable "tenant_id" {
  description = "BaseMachinaテナントID"
  type        = string
}

variable "fetch_interval" {
  description = "認可処理の公開鍵更新間隔（例: '1h', '30m'）"
  type        = string
  default     = "1h"
}

variable "fetch_timeout" {
  description = "認可処理の公開鍵更新タイムアウト（例: '10s', '30s'）"
  type        = string
  default     = "10s"
}

variable "port" {
  description = "Bridgeのリスニングポート"
  type        = number
  default     = 8080
}

# ========================================
# リソース設定
# ========================================

variable "cpu" {
  description = "Fargateタスクに割り当てるCPUユニット（256, 512, 1024, 2048, 4096）"
  type        = number
  default     = 256
}

variable "memory" {
  description = "Fargateタスクに割り当てるメモリ（MB）"
  type        = number
  default     = 512
}

variable "desired_count" {
  description = "実行するタスクの数"
  type        = number
  default     = 1
}

variable "log_retention_days" {
  description = "CloudWatch Logsの保持期間（日）"
  type        = number
  default     = 7
}

# ========================================
# タグ付けと命名
# ========================================

variable "tags" {
  description = "全リソースに適用するタグ"
  type        = map(string)
  default = {
    Environment = "production"
    Project     = "basemachina-bridge"
    ManagedBy   = "terraform"
  }
}

variable "name_prefix" {
  description = "リソース名のプレフィックス"
  type        = string
  default     = "prod"
}

# ========================================
# Bastion Host設定
# ========================================

variable "enable_bastion" {
  description = "Bastionホストを作成するかどうか。プライベートサブネット内のRDSに接続する場合に有効化します。"
  type        = bool
  default     = true
}

variable "bastion_instance_type" {
  description = "BastionホストのEC2インスタンスタイプ"
  type        = string
  default     = "t3.micro"
}

variable "bastion_ssh_public_key" {
  description = "Bastionホストへのアクセスに使用するSSH公開鍵。未指定の場合はSession Manager経由でのみアクセス可能。"
  type        = string
  default     = ""
}

variable "bastion_allowed_ssh_cidrs" {
  description = "BastionホストへのSSHアクセスを許可するCIDRブロックのリスト。bastion_ssh_public_keyが指定されている場合のみ有効。"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

# ========================================
# ドメイン設定（必須）
# ========================================

variable "bridge_domain_name" {
  description = <<-EOT
    Bridgeのドメイン名（例: bridge.example.com）。

    ACM証明書がDNS検証で自動発行され、DNS検証レコードとALBへのAレコードが
    既存のRoute53 Hosted Zoneに自動的に作成されます。
  EOT
  type        = string
}

variable "route53_zone_id" {
  description = <<-EOT
    既存のRoute53 Hosted Zone ID（必須）。

    ACM証明書のDNS検証レコードとALBへのAレコードが、このZoneに作成されます。

    Zone IDの確認方法:
      aws route53 list-hosted-zones --query "HostedZones[?Name=='example.com.'].Id" --output text
  EOT
  type        = string
}
