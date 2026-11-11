# V4.0 — perguntas antes de projetar

Uma engine unificada pode oferecer coesão, mas a avaliação da V3.6 não demonstra supremacia da arquitetura. V4.0 é apenas uma investigação futura; não há contrato de implementação nem promoção aprovada.

1. Mapear V1, V2, V3, V3.5 e V3.6 por comportamento observável, custo, invariantes, dependências, compatibilidade e evidência de utilidade. Distinguir capacidades implementadas de resultados medidos.
2. Investigar perdas de recall e falsos positivos por caso, separando gate, seleção, expansão, planejamento e entrega. Corrigir validade das métricas antes de otimizar.
3. Definir uma interface mínima e limites explícitos para recuperação, evidência, planejamento e transporte; avaliar reutilização da arquitetura existente e alternativas com testes e ablações.
4. Fixar corpus independente, métricas, controles, orçamento e critérios antes dos experimentos. Medir resultado correto, custo total, latência, privacidade e degradação por linguagem e cliente. Não usar holdout exposto como cego.
5. Só decidir padrão e migração segundo [engine-progression.md](../engine-progression.md), preservando rollback e versões existentes. Ganho offline isolado não é ganho agentic.
