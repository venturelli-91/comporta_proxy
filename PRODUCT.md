# Product

## Register

product

## Users
Quem cuida de custo e risco de IA numa empresa (engenharia de plataforma, tech leads) e precisa ver, de
relance, quem está usando os modelos, quanto custa, se o provedor está saudável e se dado pessoal está
saindo. Hoje, também o recrutador e o entrevistador que abrem a demo pública
(https://comporta.onrender.com) pela primeira vez e têm poucos minutos para entender o produto.

## Product Purpose
A Comporta é um gateway de LLM: fica entre as aplicações e os provedores de IA e controla chaves por
cliente, limite e orçamento, cache e deduplicação, resiliência (retry, circuit breaker, fallback), modo
shadow para migrar de modelo e mascaramento de dados pessoais brasileiros (LGPD). O painel é onde isso
fica visível. Sucesso: em 10 segundos o visitante entende que o provedor recebeu `[CPF_1]` e o usuário
recebeu o CPF de volta; em 1 minuto entende custo, cache e saúde dos provedores.

## Brand Personality
Confiável, preciso, calmo. Tom de ferramenta de operação séria: números em primeiro plano, frases curtas
e diretas em português, sem exclamação nem marketing. A cor existe para dar estado (ok, alerta, erro),
não para decorar.

## Anti-references
- Template de SaaS: cartões genéricos com gradiente, números gigantes de marketing, ícone + título +
  texto repetidos em grade.
- Glassmorphism decorativo, texto em gradiente, eyebrows em caixa alta acima de cada seção.
- Painel "gamer" escuro e neon só porque ferramenta técnica "parece legal" escura.

Referências positivas: Grafana e Datadog (observabilidade densa, estado em destaque) e Vercel e Linear
(tipografia firme, dark mode cuidado, interação precisa).

## Design Principles
1. **Mostre, não diga:** o mascaramento LGPD aparece como antes/depois real, não como texto explicando.
2. **Números são o conteúdo:** hierarquia vem do dado (valor grande, rótulo discreto), não de enfeite.
3. **Cor é estado:** teal para normal/ação, âmbar para alerta, vermelho para erro; o resto é neutro.
4. **Clicável parece clicável:** todo elemento interativo tem ponteiro, hover e foco visíveis.
5. **Calma sob carga:** atualização a cada 5 s sem pulos de layout; carregamento com esqueleto.

## Accessibility & Inclusion
WCAG 2.2 AA: contraste 4,5:1 para texto (3:1 para texto grande e componentes), navegação completa por
teclado com foco visível, `prefers-reduced-motion` respeitado, estado nunca comunicado só por cor
(rótulo junto), tema claro, escuro ou do sistema à escolha do usuário.
