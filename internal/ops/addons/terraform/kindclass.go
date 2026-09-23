package terraform

import (
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// kindPrefixes maps a common resource-type prefix to its blast-radius
// KindClass — the plan's own "common resource types," not an
// exhaustive provider catalogue (the phase-21 plan's own §1.1.9);
// extending this table for more providers/resource families is future
// work. Order matters only in that every prefix here is checked before
// the workload default; prefixes are otherwise independent (no two are
// expected to overlap).
var kindPrefixes = []struct {
	prefix string
	class  ops.KindClass
}{
	{"aws_db_", ops.KindData},
	{"aws_rds_", ops.KindData},
	{"aws_dynamodb_", ops.KindData},
	{"aws_s3_bucket", ops.KindData},
	{"aws_efs_", ops.KindData},
	{"aws_elasticache_", ops.KindData},
	{"google_sql_", ops.KindData},
	{"google_storage_bucket", ops.KindData},

	{"aws_iam_", ops.KindIAM},
	{"google_project_iam", ops.KindIAM},
	{"azurerm_role_", ops.KindIAM},

	{"aws_secretsmanager_", ops.KindSecret},
	{"aws_kms_", ops.KindSecret},
	{"google_secret_manager_", ops.KindSecret},

	{"aws_vpc", ops.KindNetwork},
	{"aws_subnet", ops.KindNetwork},
	{"aws_security_group", ops.KindNetwork},
	{"aws_route", ops.KindNetwork},
	{"aws_lb", ops.KindNetwork},
	{"aws_alb", ops.KindNetwork},
	{"aws_nat_", ops.KindNetwork},
	{"aws_internet_gateway", ops.KindNetwork},

	{"aws_codepipeline", ops.KindPipeline},
	{"aws_codebuild", ops.KindPipeline},
}

// classifyKind maps resourceType (a plan's own "type" field, e.g.
// "aws_instance") to a KindClass by first-matching prefix, defaulting
// to workload — the capability matrix's own terraform row names no
// crd_or_namespace resources (that class is kubectl's, phase 22).
// First-match, not longest-match: kindPrefixes' own doc comment
// declares its entries independent, no two expected to overlap, so
// there is never a second, longer match to prefer.
func classifyKind(resourceType string) ops.KindClass {
	for _, p := range kindPrefixes {
		if strings.HasPrefix(resourceType, p.prefix) {
			return p.class
		}
	}
	return ops.KindWorkload
}
