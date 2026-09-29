package event

import (
	"strings"
	"testing"
	"time"
)

func TestNearDuplicateMatcherMatchesEditedChineseReprint(
	t *testing.T,
) {
	base := strings.Join(
		[]string{
			"贵州茅台发布经营情况公告，公司表示核心产品销售保持稳定，经销渠道库存维持合理水平。",
			"公告显示，本季度营业收入继续增长，直营渠道和批发代理渠道均保持正常经营节奏。",
			"公司同时披露生产经营未发生重大变化，主要原材料采购、产品生产和市场销售正常。",
			"管理层表示将继续推进渠道结构优化，强化终端服务能力，并根据市场需求合理安排投放。",
			"公告提醒投资者关注宏观经济、消费需求以及行业竞争变化可能带来的经营风险。",
			"相关财务数据最终以公司定期报告披露的数据为准，投资者应注意投资风险。",
		},
		"",
	)
	edited := strings.Join(
		[]string{
			"财联社今日获悉，贵州茅台最新披露经营信息。",
			"贵州茅台发布经营情况公告，公司表示核心产品销售保持稳定，经销渠道库存维持合理水平。",
			"公告显示，本季度营业收入继续增长，直营渠道和批发代理渠道均保持正常经营节奏。",
			"公司同时披露生产经营未发生重大变化，主要原材料采购、产品生产和市场销售正常。",
			"管理层表示将继续推进渠道结构优化，强化终端服务能力，并根据市场需求合理安排投放。",
			"相关财务数据最终以公司定期报告披露的数据为准，投资者应注意投资风险。",
		},
		"",
	)

	left := nearDuplicateTestDocument(
		"doc-official",
		"cninfo.com.cn",
		base,
	)
	right := nearDuplicateTestDocument(
		"doc-reprint",
		"example.com",
		edited,
	)

	relation, matched := (NearDuplicateMatcher{}).Match(
		left,
		right,
	)
	if !matched {
		t.Fatalf(
			"edited reprint did not match: %+v",
			relation,
		)
	}
	if relation.Kind != EvidenceRelationNearReprint {
		t.Fatalf(
			"kind = %q, want %q",
			relation.Kind,
			EvidenceRelationNearReprint,
		)
	}
	if relation.Containment <
		DefaultNearDuplicateContainment {
		t.Fatalf(
			"containment = %.4f",
			relation.Containment,
		)
	}
	if relation.Jaccard <
		DefaultNearDuplicateJaccard {
		t.Fatalf(
			"jaccard = %.4f",
			relation.Jaccard,
		)
	}
}

func TestNearDuplicateMatcherIgnoresWhitespaceAndPunctuation(
	t *testing.T,
) {
	body := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	formatted := strings.Repeat(
		"公司公告显示：经营保持稳定；渠道库存合理，销售计划正常推进。投资者应关注相关风险！\n",
		5,
	)

	relation, matched := (NearDuplicateMatcher{}).Match(
		nearDuplicateTestDocument(
			"doc-a",
			"a.example",
			body,
		),
		nearDuplicateTestDocument(
			"doc-b",
			"b.example",
			formatted,
		),
	)
	if !matched {
		t.Fatalf(
			"formatted reprint did not match: %+v",
			relation,
		)
	}
}

func TestNearDuplicateMatcherDoesNotMatchIndependentSameEventArticles(
	t *testing.T,
) {
	left := strings.Join(
		[]string{
			"贵州茅台今日披露季度经营数据，营业收入同比增长，公司称产品销售和渠道库存总体稳定。",
			"公告重点列示了营业收入、利润以及不同销售渠道的经营数据，并提示最终数据以定期报告为准。",
			"公司表示生产经营正常，未发生影响持续经营能力的重大事项，未来将继续优化产品和渠道结构。",
			"投资者需要关注消费需求变化、市场竞争以及宏观环境波动带来的不确定性。",
		},
		"",
	)
	right := strings.Join(
		[]string{
			"白酒板块今日盘中震荡，贵州茅台股价随行业指数波动，市场交易活跃度较上一交易日有所变化。",
			"多家机构认为近期板块表现受到消费预期、资金风格和估值水平共同影响，短期走势仍存在分歧。",
			"分析人士讨论了贵州茅台的品牌优势和长期现金流，同时指出行业库存消化速度仍需持续观察。",
			"二级市场价格变化并不等同于公司基本面变化，投资者应结合公开信息独立判断风险。",
		},
		"",
	)

	relation, matched := (NearDuplicateMatcher{}).Match(
		nearDuplicateTestDocument(
			"doc-report",
			"a.example",
			left,
		),
		nearDuplicateTestDocument(
			"doc-analysis",
			"b.example",
			right,
		),
	)
	if matched {
		t.Fatalf(
			"independent articles matched: %+v",
			relation,
		)
	}
}

func TestNearDuplicateMatcherRejectsShortDocuments(
	t *testing.T,
) {
	relation, matched := (NearDuplicateMatcher{}).Match(
		nearDuplicateTestDocument(
			"doc-a",
			"a.example",
			"贵州茅台发布公告经营正常。",
		),
		nearDuplicateTestDocument(
			"doc-b",
			"b.example",
			"贵州茅台发布公告，经营正常！",
		),
	)
	if matched {
		t.Fatalf(
			"short documents matched: %+v",
			relation,
		)
	}
}

func TestNearDuplicateMatcherRejectsDistantTemplateReuse(
	t *testing.T,
) {
	body := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	left := nearDuplicateTestDocument(
		"doc-old",
		"a.example",
		body,
	)
	right := nearDuplicateTestDocument(
		"doc-new",
		"b.example",
		"最新公告导语"+body,
	)
	oldTime := time.Date(
		2026, 1, 1, 9, 0, 0, 0, time.UTC,
	)
	newTime := oldTime.Add(30 * 24 * time.Hour)
	left.PublishedAt = cloneTime(&oldTime)
	right.PublishedAt = cloneTime(&newTime)

	relation, matched := (NearDuplicateMatcher{}).Match(
		left,
		right,
	)
	if matched {
		t.Fatalf(
			"distant template reuse matched: %+v",
			relation,
		)
	}
}

func TestNearDuplicateMatcherSkipsSameSourceFuzzyRelation(
	t *testing.T,
) {
	base := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	edited := "新增导语" + base

	relation, matched := (NearDuplicateMatcher{}).Match(
		nearDuplicateTestDocument(
			"doc-a",
			"same.example",
			base,
		),
		nearDuplicateTestDocument(
			"doc-b",
			"same.example",
			edited,
		),
	)
	if matched {
		t.Fatalf(
			"same-source fuzzy relation matched: %+v",
			relation,
		)
	}
}

func TestNearDuplicateMatcherExactHashStillAudited(
	t *testing.T,
) {
	left := corroborationTestDocument(
		"doc-a",
		"a.example",
		"same-hash",
	)
	right := corroborationTestDocument(
		"doc-b",
		"b.example",
		"same-hash",
	)

	relation, matched := (NearDuplicateMatcher{}).Match(
		left,
		right,
	)
	if !matched ||
		relation.Kind != EvidenceRelationExactReprint ||
		relation.Method != "content_hash" ||
		relation.Score != 1 {
		t.Fatalf(
			"exact relation = %+v matched=%v",
			relation,
			matched,
		)
	}
}

func TestEvidenceCorroboratorNearReprintPreservesIndependentFollowup(
	t *testing.T,
) {
	base := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	edited := "媒体导语介绍最新公告内容" + base

	original := nearDuplicateTestDocument(
		"doc-a-original",
		"a.example",
		base,
	)
	reprint := nearDuplicateTestDocument(
		"doc-b-reprint",
		"b.example",
		edited,
	)
	followup := nearDuplicateTestDocument(
		"doc-b-followup",
		"b.example",
		strings.Repeat(
			"二级市场今日震荡机构讨论估值资金流向行业需求变化后续仍需观察公司公开披露",
			5,
		),
	)

	result := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{
			original,
			reprint,
			followup,
		},
	)

	if len(result.Clusters) != 2 {
		t.Fatalf(
			"clusters = %d, want 2: %+v",
			len(result.Clusters),
			result.Clusters,
		)
	}
	if len(result.Relations) != 1 ||
		result.Relations[0].Kind !=
			EvidenceRelationNearReprint {
		t.Fatalf(
			"relations = %+v",
			result.Relations,
		)
	}
	if result.IndependentSourceCount != 2 {
		t.Fatalf(
			"independent sources = %d, want 2",
			result.IndependentSourceCount,
		)
	}

	hasNearCluster := false
	for _, cluster := range result.Clusters {
		if cluster.Kind == EvidenceClusterNearReprint {
			hasNearCluster = true
		}
	}
	if !hasNearCluster {
		t.Fatalf(
			"clusters = %+v, want near-reprint cluster",
			result.Clusters,
		)
	}
}

func TestEvidenceCorroboratorRelationsDeterministicAcrossInputOrder(
	t *testing.T,
) {
	base := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	first := nearDuplicateTestDocument(
		"doc-a",
		"a.example",
		base,
	)
	second := nearDuplicateTestDocument(
		"doc-b",
		"b.example",
		"新增导语"+base,
	)

	left := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{first, second},
	)
	right := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{second, first},
	)

	if len(left.Relations) != 1 ||
		len(right.Relations) != 1 {
		t.Fatalf(
			"left=%+v right=%+v",
			left.Relations,
			right.Relations,
		)
	}
	if left.Relations[0] != right.Relations[0] {
		t.Fatalf(
			"relations differ: %+v vs %+v",
			left.Relations[0],
			right.Relations[0],
		)
	}
	if len(left.Clusters) != 1 ||
		len(right.Clusters) != 1 ||
		left.Clusters[0].ClusterID !=
			right.Clusters[0].ClusterID {
		t.Fatalf(
			"clusters differ: %+v vs %+v",
			left.Clusters,
			right.Clusters,
		)
	}
}

func nearDuplicateTestDocument(
	id string,
	domain string,
	content string,
) CanonicalDocument {
	return CanonicalDocument{
		ID:      id,
		Content: content,
		ContentHash: contentHash(
			strings.TrimSpace(content),
		),
		Source: DocumentSource{
			Publisher:       domain,
			URL:             "https://" + domain + "/" + id,
			CanonicalDomain: domain,
		},
	}
}
