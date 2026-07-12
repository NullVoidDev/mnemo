# Mnemo

> Camada de memória portátil, aberta e compartilhável para LLMs locais pequenos. Torne modelos fracos inteligentes com um arquivo `.mind`.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](go.mod)

🇺🇸 [Read in English](README.md)

## O Problema

LLMs locais pequenos (1B–3B de parâmetros — LiquidAI LFM2.5-1.2B, Qwen pequenos, etc.) rodam em qualquer PC sem GPU boa. Mas são limitados e não lembram de **nada** entre sessões. Fine-tuning local é inviável para a maioria das pessoas.

Toda conversa começa do zero. Suas correções, suas preferências, seu contexto — tudo se perde.

## A Solução

O Mnemo é um daemon local que fica entre você e o modelo — um proxy compatível com a API OpenAI que funciona com Ollama, llama.cpp, LM Studio e qualquer outra coisa que fale o mesmo protocolo. Ele injeta memórias relevantes no contexto automaticamente.

**O modelo continua o mesmo. O sistema fica cada vez mais inteligente.**

## O Formato `.mind` — o coração do projeto

Um arquivo de memória aberto, portátil e independente de modelo. Pense no que o `.gguf` foi para pesos — o `.mind` é o mesmo para memória.

Com um arquivo `.mind` você pode:

- **Trocar de modelo** (LFM → Qwen → qualquer um) e manter toda a sua "mente"
- **Exportar** sua memória como arquivo e compartilhar com outras pessoas
- **Importar** pacotes de conhecimento temáticos criados pela comunidade
- Controlar privacidade com flags público/privado por memória

Veja o rascunho da especificação: [docs/MIND_FORMAT.md](docs/MIND_FORMAT.md)

## Três Camadas de Memória

| Camada | O que armazena | Exemplo |
| ------ | -------------- | ------- |
| **Fatos** | Informações extraídas e confirmadas pelo usuário | "meu nome é X", "meu projeto usa Go" |
| **Episódios** | Resumos de conversas passadas | "em 10 de julho depuramos o timeout do proxy" |
| **Procedimentos** | Correções e preferências | "quando eu pedir X, faça Y" |

## Aprendizado com Humano no Loop

Após cada sessão, o Mnemo extrai candidatos a memória e o usuário aprova ou rejeita via CLI. **Não há auto-aprendizado automático na v1** — modelos de 1.2B não são confiáveis para auto-avaliação.

## Arquitetura (v1 / MVP)

- **Linguagem:** Go — binário único, roda em qualquer lugar, sem dependências
- **Armazenamento:** SQLite — arquivo único, fácil de exportar
- **Embeddings:** locais, em CPU, via modelo pequeno de embedding
- **Interface:** CLI + daemon HTTP. Nada de interface gráfica na v1.
- **Hardware-alvo:** um PC fraco sem GPU. Se funciona bem com o LFM2.5-1.2B via Ollama/llama.cpp, funciona com qualquer coisa.

## Licença

[MIT](LICENSE) — qualquer pessoa pode baixar, modificar e usar.
