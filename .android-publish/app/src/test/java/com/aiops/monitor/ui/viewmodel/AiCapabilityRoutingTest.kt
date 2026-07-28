package com.aiops.monitor.ui.viewmodel

import com.aiops.monitor.data.models.ObservabilityDataSource
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AiCapabilityRoutingTest {
    @Test
    fun currentServiceFailureDoesNotFallBackToToollessChat() {
        assertTrue(shouldUseLegacyAiEndpoint(404))
        assertTrue(shouldUseLegacyAiEndpoint(405))
        assertFalse(shouldUseLegacyAiEndpoint(503))
        assertFalse(shouldUseLegacyAiEndpoint(504))
    }

    @Test
    fun documentAndMcpQuestionsRequireKnowledgeSearch() {
        val prompt = buildCapabilityAwarePrompt("查询我上传的 MCP 接入文档", emptyList())

        assertTrue(prompt.contains("search_similar_cases"))
        assertTrue(prompt.contains("不得用通用知识冒充检索结果"))
    }

    @Test
    fun observabilityQuestionsCarryConnectedSourceContext() {
        val prompt = buildCapabilityAwarePrompt(
            "从 Loki 日志和 Prometheus 指标分析故障",
            listOf(
                ObservabilityDataSource(id = "1", name = "生产日志", type = "loki"),
                ObservabilityDataSource(id = "2", name = "核心指标", type = "prometheus")
            )
        )

        assertTrue(prompt.contains("list_datasources"))
        assertTrue(prompt.contains("query_datasource"))
        assertTrue(prompt.contains("生产日志(loki)"))
        assertTrue(prompt.contains("核心指标(prometheus)"))
    }

    @Test
    fun internalServiceNameIsNeverExposedInAiCopy() {
        val sanitized = sanitizeAiText("Hermes Agent 未启用，hermes 暂不可用")

        assertFalse(sanitized.contains("hermes", ignoreCase = true))
        assertTrue(sanitized.contains("智能运维服务"))
    }
}
