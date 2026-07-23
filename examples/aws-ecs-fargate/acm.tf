# ========================================
# ACM Certificate Import (Optional)
# ========================================
#
# This resource imports a self-signed certificate into AWS ACM
# for testing purposes. This is optional and only used when
# enable_acm_import is set to true.
#
# IMPORTANT: Self-signed certificates are for TESTING ONLY.
# For production environments, use certificates from a trusted CA.

resource "aws_acm_certificate" "self_signed" {
  count = var.enable_acm_import ? 1 : 0

  private_key       = var.enable_acm_import ? file("${path.module}/certs/private-key.pem") : null
  certificate_body  = var.enable_acm_import ? file("${path.module}/certs/certificate.pem") : null
  certificate_chain = var.enable_acm_import ? file("${path.module}/certs/certificate-chain.pem") : null

  tags = merge(
    var.tags,
    {
      Name        = "${var.name_prefix}-bridge-self-signed-cert"
      Environment = "testing"
      Purpose     = "self-signed-certificate-for-testing"
    }
  )

  lifecycle {
    create_before_destroy = true
  }
}
