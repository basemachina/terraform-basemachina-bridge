# ========================================
# Application Load Balancer
# ========================================
# インターネット向けのALBをパブリックサブネットに作成
# - HTTPS終端とトラフィックルーティングを提供
# - ACM証明書によるTLS 1.2以上の暗号化
# - IPホワイトリストによるアクセス制御

# BaseMachinaからHTTPSで到達できる必要があるため意図的にpublic（アクセス元はSGでBaseMachina IPに制限）
#trivy:ignore:AVD-AWS-0053
resource "aws_lb" "main" {
  name               = "${var.name_prefix}basemachina-bridge"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.public_subnet_ids

  enable_deletion_protection = false
  drop_invalid_header_fields = true

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}basemachina-bridge"
    }
  )
}

# ========================================
# ALBターゲットグループ
# ========================================
# Bridgeタスクをターゲットとするターゲットグループ
# - ターゲットタイプ: ip（Fargate対応）
# - ヘルスチェック: /okエンドポイントでHTTP 200を確認

resource "aws_lb_target_group" "bridge" {
  name        = "${var.name_prefix}bridge-tg"
  port        = var.port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc_id

  health_check {
    enabled             = true
    healthy_threshold   = 2
    unhealthy_threshold = 2
    timeout             = 5
    interval            = 30
    path                = "/ok"
    protocol            = "HTTP"
    matcher             = "200"
  }

  tags = var.tags
}

# ========================================
# ALB HTTPSリスナー
# ========================================
# HTTPS:443でリクエストを受け付け、Bridgeターゲットグループにルーティング
# - TLS 1.3/1.2の暗号化を強制
# - ACM証明書による自動TLS終端

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.main.arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = var.ssl_policy
  certificate_arn   = var.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.bridge.arn
  }
}
