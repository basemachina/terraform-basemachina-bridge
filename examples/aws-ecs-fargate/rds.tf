# ========================================
# RDS PostgreSQL Instance
# ========================================
# Bridge接続テスト用のRDSインスタンス
# init.sqlを使用してシードデータを投入

# ========================================
# RDS Security Group
# ========================================
# BridgeからのPostgreSQL接続を許可

resource "aws_security_group" "rds" {
  name_prefix = "${var.name_prefix}-rds-"
  description = "Security group for RDS PostgreSQL (allows access from Bridge)"
  vpc_id      = var.vpc_id

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-rds"
    }
  )

  lifecycle {
    create_before_destroy = true
  }
}

# BridgeからのPostgreSQL接続を許可
resource "aws_security_group_rule" "rds_ingress_from_bridge" {
  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  source_security_group_id = module.basemachina_bridge.bridge_security_group_id
  description              = "PostgreSQL from Bridge"
  security_group_id        = aws_security_group.rds.id
}

# RDSからのアウトバウンドルール（全トラフィック許可）
#trivy:ignore:AVD-AWS-0104
resource "aws_security_group_rule" "rds_egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "All outbound traffic"
  security_group_id = aws_security_group.rds.id
}

# ========================================
# RDS Subnet Group
# ========================================
# RDSインスタンスを配置するサブネットグループ

resource "aws_db_subnet_group" "main" {
  name_prefix = "${var.name_prefix}-rds-"
  description = "Subnet group for RDS PostgreSQL instance"
  subnet_ids  = var.private_subnet_ids

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-rds-subnet-group"
    }
  )

  lifecycle {
    create_before_destroy = true
  }
}

# ========================================
# Random Password for RDS
# ========================================
# RDSマスターパスワードをランダム生成

resource "random_password" "rds_master_password" {
  length  = 32
  special = true
  # PostgreSQLで使用できない特殊文字を除外
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# ========================================
# RDS PostgreSQL Instance
# ========================================
# Bridge接続テスト用のPostgreSQLインスタンス

# 接続テスト用の簡易DBのため、削除保護・IAM認証・CMKによるPerformance Insights暗号化は省略
#trivy:ignore:AVD-AWS-0176
#trivy:ignore:AVD-AWS-0177
#trivy:ignore:AVD-AWS-0078
resource "aws_db_instance" "postgres" {
  identifier_prefix = "${var.name_prefix}-bridge-example-"

  # Engine configuration
  engine         = "postgres"
  engine_version = "16.4"
  instance_class = "db.t3.micro"

  # Storage configuration
  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type          = "gp3"
  storage_encrypted     = true

  # Database configuration
  db_name  = "bridge_example"
  username = "postgres"
  password = random_password.rds_master_password.result
  port     = 5432

  # Network configuration
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  # Backup configuration
  backup_retention_period = 7
  backup_window           = "03:00-04:00"
  maintenance_window      = "mon:04:00-mon:05:00"

  # Performance and monitoring
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  monitoring_interval             = 60
  monitoring_role_arn             = aws_iam_role.rds_monitoring.arn
  performance_insights_enabled    = true

  # High availability and recovery
  multi_az                   = false # テスト環境のためシングルAZ
  deletion_protection        = false # テスト環境のため削除保護は無効
  skip_final_snapshot        = true  # テスト環境のため最終スナップショットはスキップ
  copy_tags_to_snapshot      = true
  apply_immediately          = true
  auto_minor_version_upgrade = true

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bridge-example-postgres"
    }
  )

  # セキュリティグループとSubnet Groupへの依存関係を明示
  depends_on = [
    aws_security_group_rule.rds_ingress_from_bridge,
    aws_security_group_rule.rds_egress_all,
    aws_db_subnet_group.main
  ]
}

# ========================================
# IAM Role for RDS Enhanced Monitoring
# ========================================

resource "aws_iam_role" "rds_monitoring" {
  name_prefix = "${var.name_prefix}-rds-monitoring-"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "monitoring.rds.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-rds-monitoring"
    }
  )
}

resource "aws_iam_role_policy_attachment" "rds_monitoring" {
  role       = aws_iam_role.rds_monitoring.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}

# ========================================
# Secrets Manager for RDS Credentials
# ========================================
# RDS接続情報をSecrets Managerに保存

# サンプル用のため暗号化はデフォルトのAWS管理キーを使用（CMK省略）
#trivy:ignore:AVD-AWS-0098
resource "aws_secretsmanager_secret" "rds_credentials" {
  name_prefix = "${var.name_prefix}-rds-credentials-"
  description = "RDS PostgreSQL connection credentials for Bridge example"

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-rds-credentials"
    }
  )
}

resource "aws_secretsmanager_secret_version" "rds_credentials" {
  secret_id = aws_secretsmanager_secret.rds_credentials.id
  secret_string = jsonencode({
    username = aws_db_instance.postgres.username
    password = random_password.rds_master_password.result
    engine   = "postgres"
    host     = aws_db_instance.postgres.address
    port     = aws_db_instance.postgres.port
    dbname   = aws_db_instance.postgres.db_name
    # Connection string format
    connection_string = "postgresql://${aws_db_instance.postgres.username}:${random_password.rds_master_password.result}@${aws_db_instance.postgres.address}:${aws_db_instance.postgres.port}/${aws_db_instance.postgres.db_name}"
  })
}

# ========================================
# Database Initialization Note
# ========================================
# RDSインスタンスはプライベートサブネット内にあるため、
# Terraformのlocal-exec provisioner からは接続できません。
#
# データベース初期化は以下のいずれかの方法で行ってください:
#
# 1. Bastionホスト経由で接続:
#    psql -h ${aws_db_instance.postgres.address} -U postgres -d bridge_example -f scripts/init.sql
#
# 2. AWS Systems Manager Session Manager経由:
#    - EC2インスタンスをプライベートサブネットに起動
#    - Session Manager でログイン
#    - psqlコマンドでinit.sqlを実行
#
# 3. ECS Execを使用してBridgeコンテナから実行:
#    aws ecs execute-command --cluster <cluster-name> --task <task-id> --container bridge --interactive --command "/bin/sh"
#    psql -h ${aws_db_instance.postgres.address} -U postgres -d bridge_example -f /path/to/init.sql
#
# RDS接続情報はSecrets Managerに保存されています:
#   Secret ARN: ${aws_secretsmanager_secret.rds_credentials.arn}
#
# 接続情報を取得:
#   aws secretsmanager get-secret-value --secret-id ${aws_secretsmanager_secret.rds_credentials.arn} --query SecretString --output text | jq .
