# V3.7 — plano de pesquisa e marcos

**Estado: proposta; nenhuma engine V3.7 implementada, resultado medido ou promoção aprovada.** Objetivo: descobrir qual combinação de recuperação automática, contexto inicial e aprofundamento opcional ajuda um agente a concluir tarefas reais corretamente, em repositórios que a política e os autores dos testes não usaram para ajuste. A unificação é uma hipótese arquitetural plausível; não é o resultado presumido. O [mapa das engines](../engines.md) separa o estado atual e as evidências.

## Perguntas e hipóteses

1. **Validade externa:** o que acontece quando o repositório, a linguagem, as convenções e os prompts são novos para a política e para quem escolheu os sinais? Hipótese: desempenho em fixtures do Sidecar superestima generalização. Medir por repositório e por classe, sem escolher só projetos ou casos favoráveis.
2. **Contexto automático:** evidência entregue no hook melhora a próxima decisão útil e a tarefa completa em relação ao mesmo agente sem Sidecar? Hipótese: código, relações e testes relevantes podem reduzir buscas ou evitar erros; evidência irrelevante, prematura ou longa pode induzir erro e consumir orçamento.
3. **Unificação:** planner/registro/packing globais e QuickExpand automático ajudam mais que o pipeline anterior, ao mesmo custo? Hipóteses alternativas: (a) a arquitetura unificada permite melhor composição e proveniência; (b) gate, seleção, limites ou implementação específica escondem evidência; (c) a unificação adiciona custo sem ganho. Uma falha da V3.5 ou V3.6 não prova que toda arquitetura unificada é ruim; tampouco prova que a unificação compensa.
4. **Aprofundamento:** quando contexto inicial não basta, o agente usa busca/leitura normais ou consegue descobrir e usar `sidecar explore`? Um hint ajuda ou desvia? A ferramenta traz informação marginal suficiente para pagar o custo? Não presumir familiaridade, permissão nem disponibilidade uniforme entre clientes.
5. **Balanceamento:** qual combinação protege recall, abstinência, cobertura de fluxo e qualidade de edição com tokens, latência e custo aceitáveis? Medir também falsos positivos, falsas abstenções, evidência desatualizada e contradições, por tarefa e tipo de repositório.

## M0 — inventário e integridade

- Fixar SHA do Sidecar, versão de cada engine, flags, hook/adaptador, cliente, modelo servido, ferramentas, permissões, máquina, concorrência e estado frio/quente. Registrar se o código está sujo; para resultados comparáveis usar revisão reproduzível, não um diff local implícito.
- Mapear capacidades por versão e por fase: retrieval, gate, snippets, expansão, orientação, sessão, memória e impacto. Confirmar no código, testes e traces; registrar conflitos entre documentos. Não concluir que um recurso foi usado por existir na arquitetura.
- Auditar scorecard, isolamento de ferramentas/hints, captura de contexto e oráculo; testar casos intencionalmente errados, timeout, silêncio e ausência de métricas. Contadores não observados são `unknown`, não zero.
- Critério de saída: outro pesquisador consegue reconstruir configurações, sabe o que cada braço pode acessar e consegue distinguir falha de harness de comportamento da engine.

## M1 — corpus externo e desenho antes de medir

- Selecionar repositórios reais de diferentes tamanhos, linguagens, estruturas e maturidade, com fontes, licenças e commits fixos; incluir sem Git, linguagens não suportadas, índice incompleto, arquivos movidos e worktrees quando pertinente. Registrar critérios e recusas sem esconder falhas. Para validar generalização, separar repositórios inteiros por desenvolvimento, validação e **novo** holdout, evitando variantes da mesma tarefa ou forks no outro split.
- Preferir tarefas reais derivadas de issues/histórico ou definidas por pessoas que não ajustam a engine; anotar antes do ensaio origem, escopo, ambiguidades e oráculo. Incluir localização, fluxo entre arquivos, debug, edição, contratos, testes, solicitações curtas/longas, PT-BR/EN, conversa, fora do repositório, pistas enganosas e informação ausente. Não exigir nomes de arquivos, comandos Sidecar ou vocabulário de benchmark em prompts que um usuário normal não forneceria.
- Verificar oráculos e testes independentemente: código, testes e documentação podem divergir; registrar proveniência e incerteza, não converter uma fonte automaticamente em verdade. Para edição usar teste que falha na base e passa em referência quando possível, testes de regressão e avaliação de escopo; não expor respostas ou testes ocultos ao agente.
- Os conjuntos internos e externos já inspecionados, inclusive o holdout de negativos exposto, são diagnóstico e não cegos. O holdout novo deve ficar inacessível a quem ajusta e só ser aberto após freeze explícito. Não gerar casos sobre as frases de validação já vistas e chamá-los independentes.
- Pré-registrar hipóteses, desfechos primários, limiares, regras de exclusão, custo máximo, tamanho de amostra/poder ou limitações, ordem balanceada, seeds e análise por tarefa/repositório. Não mudar critérios após olhar resultados para fazer uma variante vencer.

## M2 — comparação offline e ablação

Reproduzir em commits fixos e mesma configuração de índice: V1, V2, V3, V3.5, V3.6 e candidatas V3.7 quando existirem. Medir separadamente:

1. **Recuperação:** candidatos e evidência relevante, recall/precisão, posição, diversidade, `unknown` e resultado vazio; conferir fontes e atualidade.
2. **Decisão:** razão de abstenção e classificação; falsos positivos e falsas abstenções por classe. Diferenciar gate que decide não recuperar de packer que não entrega.
3. **Entrega:** contexto efetivamente emitido, caminho e símbolo no mesmo item quando aplicável, cobertura de fatos verificados, snippets e relações, proveniência, duplicação, perda entre planejado/entregue, orçamento e injeção insegura. `Gold` por caminho isolado é diagnóstico, não satisfação da tarefa.
4. **Custo:** tempo de indexação/sincronização e hook, memória, tokens estimados e realmente observados, cache e falhas. Medir distribuição com repetições, cold/warm e concorrência controlada; uma execução única não sustenta ranking de latência.

Ablações devem ser comparações pareadas que mudam um fator por vez: gate, seleção/ordenamento, planner+packer, brief, QuickExpand, hint e providers. Usar switches existentes quando possível; uma ablação ainda não implementada é apenas proposta, não novo recurso presumido. Avaliar interações relevantes depois das principais; não comparar braços com recursos ou orçamentos acidentalmente diferentes. Investigar pioras por caso e agrupar por causa antes de qualquer ajuste.

## M3 — piloto agentic com ferramentas equilibradas

Somente após integridade e com autorização específica para custos pagos, comparar nos **mesmos** repositórios/tarefas, cliente, modelo, permissões, orçamento, isolamento e ferramentas normais:

| Braço | Condição | Pergunta |
|---|---|---|
| OFF | Sem hook nem acesso acidental ao Sidecar; ferramentas normais disponíveis. | Como o agente resolve sozinho? |
| Engines via hook | V1, V2, V3, V3.5, V3.6 e, quando existir, V3.7; sem exigir comando Sidecar. | O contexto automático ajuda ou prejudica? |
| Exploração opcional | Mesmo hook, com `explore` disponível e permitido, sem dizer ao agente como usá-lo. | Ele descobre e usa espontaneamente? |
| Exploração com hint | Mesmo braço anterior, com hint explícito e custo contabilizado. | A orientação muda descoberta, uso e resultado? |
| Controle de ferramenta | Acesso equivalente a busca/leitura nativas e, se adequado, alternativas de navegação com permissão e instrução equivalentes. | A diferença vem do contexto, da ferramenta adicional ou da familiaridade? |

Os braços acima são desenho-alvo: não supor que o harness atual fornece isolamento, capacidade de desligar cada hint ou controle de permissão. Validar antes; marcar indisponível quando não der para igualar. Para uma ablação causal, comparar mesmo motor com e sem expansão automática e depois com/sem hint/ferramenta; não confundir `sidecar explore` executado explicitamente em benchmark offline com uso autônomo do agente.

Medir desfechos primários **tarefa corretamente concluída com invariantes e escopo**, custo total por tarefa correta e tempo total; incluir todos os fracassos e overhead de indexação, hook, tokens de contexto, ferramenta e chamadas de modelo. Secundários: fonte efetivamente usada, citações corretas, primeira ação substantiva após o contexto, buscas/leitura por arquivo já entregue, ferramenta descoberta/invocada, erros de entendimento, retrabalho e tokens por origem. O comportamento observado não revela diretamente o “pensamento” interno do modelo: interpretar como ações e resultados observáveis, sem inferir raciocínio oculto. Considerar variação por modelo/cliente e reportar incerteza com pareamento e agrupamento por repositório; um piloto pequeno informa viabilidade, não prova produtividade.

## M4 — decisão V3.7 e eventual V4

Somente após M0–M3: selecionar o problema com evidência, escrever contrato e regressões independentes, implementar mudança mínima local-first, comparar com baseline e ablações e publicar resultados bons e ruins. Manter engines e rollback; usar os [critérios de progressão](../engine-progression.md), sem promover um padrão porque uma métrica isolada melhorou. Se as evidências forem insuficientes, manter a decisão inconclusiva.

V4 é uma possível fase posterior para consolidar a arquitetura à luz dos resultados, não uma reescrita aprovada. A questão principal é qual responsabilidade deve ser automática, opcional ou deixada ao agente, com fronteiras e custos explícitos. Ver [v4-discovery.md](../v3.6/v4-discovery.md).

## Disciplina operacional

Sem LLM/rede no runtime de recuperação, sem executar código do repositório para indexá-lo, sem mudar prompt nem guardar prompts/código em telemetria. Hook falha aberto; orçamento de tempo/memória/tokens é explícito. Instalação e observação são consentidas e reversíveis; ferramentas e intervenções dependem do cliente realmente verificado. Downloads, corpus e testes com agentes são avaliação externa opcional, não dependências do produto.
