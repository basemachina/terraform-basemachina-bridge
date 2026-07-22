# ========================================
# タスク実行ロール (Task Execution Role)
# ========================================
# ECS Fargateがタスクを起動する際に必要な権限を提供
# - ECRイメージのpull
# - CloudWatch Logsへの書き込み

resource "aws_iam_role" "task_execution" {
  name_prefix = "${local.prefixed_app_name}-execution-"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = var.tags
}

# AmazonECSTaskExecutionRolePolicyマネージドポリシーのアタッチ
resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# AmazonEC2ContainerRegistryReadOnlyマネージドポリシーのアタッチ
# ECR (プライベート/パブリック) からイメージをpullするために必要
# （プルスルーキャッシュを使わずpublic.ecr.awsから直接pullする場合は不要）
resource "aws_iam_role_policy_attachment" "ecr_read_only" {
  count = var.enable_ecr_pull_through_cache ? 1 : 0

  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# CloudWatch Logsへの書き込み権限を持つインラインポリシー
resource "aws_iam_role_policy" "cloudwatch_logs" {
  name = "cloudwatch-logs"
  role = aws_iam_role.task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "${aws_cloudwatch_log_group.bridge.arn}:*"
      }
    ]
  })
}

