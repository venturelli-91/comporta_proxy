# Comporta — for non-technical readers

## In one sentence
Comporta is a "front desk" between a company's systems and artificial intelligence services: it controls who
uses them, how much is spent and which personal data leaves the company.

## The problem it solves
When several teams start using AI, four things happen:
1. **Runaway cost:** nobody knows how much each team spends, and one faulty system burns the month's budget.
2. **Paying twice for the same answer:** the same question is sent and billed many times.
3. **Dependence on one vendor:** if the AI service goes down, everything goes down with it.
4. **Privacy risk (LGPD, Brazil's data protection law):** customers' CPF (taxpayer ID), email and phone numbers end up sent to companies outside Brazil.

## What Comporta does
- Each system gets its own "credential", with a usage limit and a monthly spend limit.
- Repeated questions are answered from memory, without paying again.
- If the vendor fails, it tries again or uses a backup vendor.
- CPF, CNPJ, email, phone and card numbers are replaced with codes before leaving, and the original data comes back in the answer.
- A dashboard shows spend, savings and how much personal data was protected.

## Limits (said honestly)
The data protection recognizes known formats. Names and addresses aren't detected. It reduces the risk, but on its own it doesn't guarantee LGPD compliance.

## Status (2026-10-09)
The MVP is ready and measured, four days ahead of the target (10/13):
- It handles **1,000 calls per second** while adding less than **2 thousandths of a second** to each one.
- When the same question repeats, the answer comes from memory: **99.9% of paid calls avoided** in the test.
- With the AI vendor **failing half the time**, **98.4%** of calls still succeeded.
- In every test, CPF and email went out replaced with codes and came back correctly to the user.

Next steps (after 10/14): run several copies at once in the cloud (Google Cloud) and budget alerts.
