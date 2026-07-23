# ========================================
# ALBセキュリティグループ
# ========================================
# Application Load BalancerへのHTTPSアクセスを制御
# - インバウンド: HTTPS:443をBaseMachina (34.85.43.93/32)から許可
# - アウトバウンド: 全トラフィックを許可

resource "aws_security_group" "alb" {
  name_prefix = "${local.prefixed_app_name}-alb-"
  description = "Security group for BaseMachina Bridge ALB"
  vpc_id      = var.vpc_id

  tags = merge(
    var.tags,
    {
      Name = "${local.prefixed_app_name}-alb"
    }
  )
}

# ALBへのHTTPSインバウンドルール（BaseMachina IP）
resource "aws_security_group_rule" "alb_ingress_https_basemachina" {
  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = ["34.85.43.93/32"]
  description       = "HTTPS from BaseMachina"
  security_group_id = aws_security_group.alb.id
}

# ALBへのHTTPSインバウンドルール（追加CIDR）
resource "aws_security_group_rule" "alb_ingress_https_additional" {
  count             = length(var.additional_alb_ingress_cidrs) > 0 ? 1 : 0
  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = var.additional_alb_ingress_cidrs
  description       = "HTTPS from additional sources (e.g., testing)"
  security_group_id = aws_security_group.alb.id
}

# ALBからのアウトバウンドルール（全トラフィック許可）
#trivy:ignore:AVD-AWS-0104
resource "aws_security_group_rule" "alb_egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "All outbound traffic"
  security_group_id = aws_security_group.alb.id
}

# ========================================
# Bridgeセキュリティグループ
# ========================================
# Bridge Fargateタスクへのアクセスを制御
# - インバウンド: var.portをALBセキュリティグループから許可
# - アウトバウンド: 全トラフィックを許可

resource "aws_security_group" "bridge" {
  name_prefix = "${local.prefixed_app_name}-task-"
  description = "Security group for BaseMachina Bridge tasks"
  vpc_id      = var.vpc_id

  tags = merge(
    var.tags,
    {
      Name = "${local.prefixed_app_name}-task"
    }
  )
}

# BridgeへのHTTPインバウンドルール（ALBからのみ）
resource "aws_security_group_rule" "bridge_ingress_http" {
  type                     = "ingress"
  from_port                = var.port
  to_port                  = var.port
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.alb.id
  description              = "HTTP from ALB"
  security_group_id        = aws_security_group.bridge.id
}

# Bridgeからのアウトバウンドルール（全トラフィック許可）
#trivy:ignore:AVD-AWS-0104
resource "aws_security_group_rule" "bridge_egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "All outbound traffic"
  security_group_id = aws_security_group.bridge.id
}

