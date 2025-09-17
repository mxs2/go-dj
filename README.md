# Go DJ

Um simples mixer de DJ baseado em linha de comando, escrito em Go. Ele permite carregar múltiplos arquivos de áudio `.wav` de um diretório, mixá-los e controlar a reprodução, volume e BPM de cada faixa individualmente em tempo real.

O desafio é criar uma aplicação de console que simula uma mesa de DJ, onde diferentes faixas musicais (representadas por instrumentos) tocam simultaneamente.

Cada "instrumento" (ou faixa de música) vai tocar em sua própria , de forma totalmente independente. O DJ (você!) poderá interagir com o sistema através de comandos de texto para controlar cada faixa individualmente, pausando e retomando a reprodução sem afetar as demais.

## Música escolhida
Nujabes - Luv(sic) feat.Shing02 

## Funcionalidades

  - **Carregamento Automático:** Carrega todos os arquivos `.wav` de um diretório `musics/` na inicialização.
  - **Controle de Reprodução:** Comandos para `play`, `pause`, `stop` (mudo) e `replay` para faixas individuais ou para todas de uma vez.
  - **Ajuste de Volume:** Altere o volume de cada instrumento de forma independente.
  - **Controle de Velocidade (BPM):** Acelere ou desacelere as faixas ajustando o BPM desejado.
  - **Mixagem em Tempo Real:** Todas as faixas são mixadas e reproduzidas simultaneamente.
  - **Interface de Linha de Comando:** Controle tudo através de comandos simples no seu terminal.
  - **Multiplataforma:** Funciona no Windows e no Linux.

## Pré-requisitos

1.  **Go:** É necessário ter o Go (versão 1.18 ou mais recente) instalado. Você pode baixá-lo em [go.dev](https://go.dev/dl/).
2.  **Arquivos de Áudio:** Você precisará de alguns arquivos de áudio no formato `.wav`.

## Como Inicializar

Siga os passos abaixo para configurar e executar o projeto.

### 1\. Obtenha o Código

Primeiro, clone o repositório para a sua máquina local usando Git. Se não tiver Git, pode baixar o código como um arquivo ZIP.

```bash
# Substitua pela URL correta do repositório, se aplicável
git clone https://github.com/seu-usuario/go-dj-mixer.git
cd go-dj-mixer
```

### 2\. Adicione seus Arquivos de Áudio

O programa procura por uma pasta chamada `musics` no mesmo diretório do executável.

  - Copie seus arquivos `.wav` para dentro da pasta `./musics/`.

A estrutura do seu projeto deve ficar assim:

```
go-dj/
├── main.go          # O arquivo de código do projeto
└── musics/
    ├── drums.wav
    ├── bass.wav
    └── synth.wav
```

### 3\. Dependências e Execução

As instruções a seguir servem tanto para Windows quanto para Linux, mas há uma nota importante para usuários Linux.

#### Windows

No Windows, o Go geralmente lida com as dependências de áudio sem necessidade de instalação adicional.

1.  **Abra um terminal** (CMD ou PowerShell) na pasta do projeto.

2.  **Instale as dependências do Go:** O comando a seguir irá baixar as bibliotecas necessárias (como `beep`).

    ```bash
    go mod tidy
    ```

3.  **Execute o programa:**

    ```bash
    go run .
    ```

#### Linux

No Linux, além dos pacotes do Go, a biblioteca de áudio `beep` precisa de pacotes de desenvolvimento de sistema para compilar. O mais comum é o `ALSA`.

1.  **Instale as dependências de áudio do sistema:**

      * Em distribuições baseadas em Debian/Ubuntu:
        ```bash
        sudo apt-get update && sudo apt-get install libasound2-dev
        ```
      * Em distribuições baseadas em Fedora/CentOS:
        ```bash
        sudo dnf install alsa-lib-devel
        ```

2.  **Abra um terminal** na pasta do projeto.

3.  **Instale as dependências do Go:**

    ```bash
    go mod tidy
    ```

4.  **Execute o programa:**

    ```bash
    go run .
    ```

Após executar `go run .` em qualquer um dos sistemas, o mixer de DJ estará ativo e pronto para receber comandos no terminal.

## Como Usar

Assim que o programa estiver em execução, você verá um prompt `>`. Digite `help` para ver a lista de comandos disponíveis.

```
--- DJ Mixer Commands ---
  play [name]      - Plays or resumes an instrument (or all).
  replay [name]    - Restarts an instrument from the beginning (or all).
  pause [name]     - Pauses an instrument at its current position (or all).
  stop [name]      - Stops an instrument by muting it (or all).
  volume <name> <v> - Sets instrument volume (-2.0 to 2.0).
  bpm <name> <v>   - Sets instrument BPM (e.g., 'bpm drums 140').
  list             - Shows the status of all instruments.
  help             - Shows this help message.
  quit             - Exits the program (or use Ctrl+C).
-------------------------
```

### Exemplos de Comandos:

  - `list`: Mostra os instrumentos carregados (ex: `drums`, `bass`).
  - `play drums`: Começa a tocar a faixa `drums.wav`.
  - `play`: Começa a tocar todas as faixas ao mesmo tempo.
  - `volume bass 0.5`: Define o volume da faixa `bass` para `0.5`.
  - `bpm drums 140`: Altera a velocidade da faixa `drums` para corresponder a 140 BPM.
  - `stop drums`: Silencia a faixa `drums` (ela continua tocando em mudo).
  - `pause`: Pausa a reprodução de todas as faixas.
  - `quit` ou `Ctrl+C`: Encerra o programa.

<hr>

Feito com ❤️ por [Mateus Xavier](https://github.com/mxs2)