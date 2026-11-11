# Progressão das engines

Para histórico, diferenças e evidências por versão, veja [engines.md](engines.md). Para o estudo proposto sobre V3.7, incluindo contexto automático versus exploração opcional, veja [research-plan.md](v3.7/research-plan.md).

As engines existentes são alternativas experimentais. O padrão atual é V1; V2, V3 e V3.5 são opt-in; a V3.6 é experimental e ainda está fora da `main`. A arquitetura unificada é uma hipótese de melhor coesão, não prova de melhor resultado. Publicar código opt-in não substitui nem promove uma engine.

Antes de substituir uma engine pelo seu sucessor, definir e congelar a matriz de benchmarks, versões e ambientes, métricas, direção de melhoria, limites, incerteza e critérios de desempate. O sucessor deve superar o anterior em **todos os benchmarks aplicáveis**, sem regredir nos critérios de correção, segurança, compatibilidade, falsos positivos, latência e custo previamente definidos. Empates, casos sem dados, diferenças não comparáveis e resultados incertos não demonstram superioridade. Manter os resultados negativos e a possibilidade de rollback. Melhor Hit@K ou menos tokens, isoladamente, não prova ganho de produtividade. A política de avaliação detalhada permanece em [evaluation-plan.md](evaluation-plan.md).

Comparações devem parear tarefas e commits de repositórios, incluir falhas e negativos, controlar ambiente e distinguir recuperação offline, entrega do hook e resultado correto de agente. Os conjuntos internos e externos já examinados são desenvolvimento ou validação consumida, não holdout cego. A seleção final exige holdout independente, freeze explícito e execução conforme o protocolo; experimentos pagos exigem autorização específica.

A V3.6 é uma candidata experimental fora da `main`, com resultados e regressões em [v3.6/progress.md](v3.6/progress.md). Os dados atuais não justificam substituir V1 ou V3.5, nem escolher uma vencedora geral. Uma possível V4.0 deve primeiro consolidar requisitos, invariantes, evidências por engine, desenho e plano de avaliação; não pressupor que a unificação já venceu os controles.
