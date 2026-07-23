# ========================================
# CloudWatch Logsロググループ
# ========================================
# Bridgeコンテナの標準出力・標準エラー出力を集約

# CloudWatch LogsはデフォルトでAWS管理キーにより暗号化されるため、CMKの強制はしない
#trivy:ignore:AVD-AWS-0017
resource "aws_cloudwatch_log_group" "bridge" {
  name              = "/ecs/${var.name_prefix}basemachina-bridge"
  retention_in_days = var.log_retention_days

  tags = var.tags
}
