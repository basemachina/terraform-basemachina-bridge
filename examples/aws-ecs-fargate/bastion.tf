# ========================================
# Bastion Host
# ========================================
# プライベートサブネット内のRDSに接続するためのBastionホスト
# SSH接続してpsqlコマンドでデータベース初期化を実行可能

# ========================================
# Data Sources
# ========================================

# 最新のAmazon Linux 2023 AMIを取得
data "aws_ami" "amazon_linux_2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# ========================================
# Bastion Security Group
# ========================================

resource "aws_security_group" "bastion" {
  count = var.enable_bastion ? 1 : 0

  name_prefix = "${var.name_prefix}-bastion-"
  description = "Security group for Bastion host (SSH access)"
  vpc_id      = var.vpc_id

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bastion"
    }
  )
}

# SSH インバウンドルール
# 許可CIDRは利用者がbastion_allowed_ssh_cidrs変数で制限する前提
#trivy:ignore:AVD-AWS-0107
resource "aws_security_group_rule" "bastion_ingress_ssh" {
  count = var.enable_bastion ? 1 : 0

  type              = "ingress"
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  cidr_blocks       = var.bastion_allowed_ssh_cidrs
  description       = "SSH from allowed CIDRs"
  security_group_id = aws_security_group.bastion[0].id
}

# アウトバウンドルール（全トラフィック許可）
#trivy:ignore:AVD-AWS-0104
resource "aws_security_group_rule" "bastion_egress_all" {
  count = var.enable_bastion ? 1 : 0

  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "All outbound traffic"
  security_group_id = aws_security_group.bastion[0].id
}

# RDSへのアクセスを許可（BastionからRDSへ）
resource "aws_security_group_rule" "rds_ingress_from_bastion" {
  count = var.enable_bastion ? 1 : 0

  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.bastion[0].id
  description              = "PostgreSQL from Bastion"
  security_group_id        = aws_security_group.rds.id
}

# ========================================
# SSH Key Pair
# ========================================

resource "aws_key_pair" "bastion" {
  count = var.enable_bastion && var.bastion_ssh_public_key != "" ? 1 : 0

  key_name_prefix = "${var.name_prefix}-bastion-"
  public_key      = var.bastion_ssh_public_key

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bastion-key"
    }
  )
}

# ========================================
# IAM Role for Bastion (Session Manager用)
# ========================================

resource "aws_iam_role" "bastion" {
  count = var.enable_bastion ? 1 : 0

  name_prefix = "${var.name_prefix}-bastion-"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bastion-role"
    }
  )
}

# Session Manager用のポリシーをアタッチ
resource "aws_iam_role_policy_attachment" "bastion_ssm" {
  count = var.enable_bastion ? 1 : 0

  role       = aws_iam_role.bastion[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "bastion" {
  count = var.enable_bastion ? 1 : 0

  name_prefix = "${var.name_prefix}-bastion-"
  role        = aws_iam_role.bastion[0].name

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bastion-profile"
    }
  )
}

# ========================================
# Bastion EC2 Instance
# ========================================

resource "aws_instance" "bastion" {
  count = var.enable_bastion ? 1 : 0

  ami           = data.aws_ami.amazon_linux_2023.id
  instance_type = var.bastion_instance_type

  # パブリックサブネットの最初のサブネットに配置
  subnet_id                   = var.public_subnet_ids[0]
  vpc_security_group_ids      = [aws_security_group.bastion[0].id]
  associate_public_ip_address = true

  # SSH Key Pair
  key_name = var.bastion_ssh_public_key != "" ? aws_key_pair.bastion[0].key_name : null

  # IAM Instance Profile (Session Manager用)
  iam_instance_profile = aws_iam_instance_profile.bastion[0].name

  # User Data: PostgreSQLクライアントをインストール
  user_data_base64 = base64encode(<<-EOF
    #!/bin/bash
    set -e

    # システムアップデート
    dnf update -y

    # PostgreSQL 15クライアントをインストール
    dnf install -y postgresql15

    # jqをインストール（Secrets Manager用）
    dnf install -y jq

    # AWS CLI v2（Amazon Linux 2023にはプリインストール済み）
    # aws --version

    echo "Bastion host setup completed" > /var/log/bastion-setup.log
  EOF
  )

  # ルートボリューム設定
  root_block_device {
    volume_type           = "gp3"
    volume_size           = 30
    encrypted             = true
    delete_on_termination = true
  }

  # メタデータオプション（IMDSv2を強制）
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  tags = merge(
    var.tags,
    {
      Name = "${var.name_prefix}-bastion"
    }
  )
}
