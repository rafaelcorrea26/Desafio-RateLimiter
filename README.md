# Desafio Rate Limiter

Desafio da pós de Go. É um rate limiter que funciona como middleware de um
servidor HTTP e limita as requisições por IP ou por token de acesso, guardando
os contadores no Redis.

Quando o cliente passa do limite, recebe status 429 com a mensagem:

```
you have reached the maximum number of requests or actions allowed within a certain time frame
```

e fica bloqueado pelo tempo configurado (5 minutos no `.env` padrão). Durante o
bloqueio todas as requisições dele levam 429, mesmo que o segundo já tenha
virado.

## Como rodar

Só precisa de Docker. Na raiz do projeto:

```
docker compose up --build
```

Isso sobe o Redis e a aplicação na porta 8080. Pra testar:

```
curl -i http://localhost:8080/
curl -i -H "API_KEY: abc123" http://localhost:8080/
```

Pra ver o 429 acontecendo, dá pra mandar uma rajada de requisições. Com o limite
padrão de 10 req/s por IP, a partir da 11ª começa a voltar 429:

```
for i in $(seq 1 15); do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/; done
```

E com o token `abc123`, que tem limite de 100 req/s, as mesmas 15 passam:

```
for i in $(seq 1 15); do curl -s -o /dev/null -w "%{http_code}\n" -H "API_KEY: abc123" http://localhost:8080/; done
```

Detalhe: depois de tomar 429 o IP fica bloqueado pelos 300 segundos do
`BLOCK_DURATION`. Se quiser testar de novo sem esperar, ou baixa esse valor no
`.env` (o mínimo é 1) e reinicia, ou limpa o Redis com
`docker compose exec redis redis-cli flushall`.

Também deixei um `test/requests.http` pra quem usa a extensão REST Client do
VS Code.

Se quiser rodar fora do Docker (precisa de Go 1.26), sobe só o Redis e roda o
servidor da raiz do projeto, que é onde ele procura o `.env`:

```
docker compose up -d redis
go run ./cmd/server
```

## Como rodar os testes

Também pelo Docker:

```
docker compose run --rm tests
```

Esse serviço só existe no profile `test`, então não sobe junto com o
`docker compose up`. Ele roda o `go test -v ./...` dentro da imagem de build,
apontando pro Redis do compose.

Com Go instalado dá pra rodar direto com `go test ./...`. Nesse caso os testes
do Redis usam o [miniredis](https://github.com/alicebob/miniredis), que é um
Redis em memória feito pra teste, e não precisa de nada rodando.

O que os testes cobrem:

- `internal/limiter`: a regra em si. Limite por IP, IPs independentes entre si,
  token sobrepondo o IP (o exemplo do enunciado, 10 req/s por IP e 100 pro
  token), token com limite menor que o IP, token desconhecido caindo no limite
  padrão, IP bloqueado não afetando o mesmo IP com token, contador zerando depois
  de 1 segundo e bloqueio durando mais que a janela.
- `internal/infra/web/middleware`: status 429 com a mensagem exata, precedência
  do token passando pelo HTTP, o header `API_KEY` mandado de verdade pela rede
  com `httptest.NewServer`, e o comportamento quando o storage cai.
- `internal/infra/storage`: o contador do Redis incrementando com TTL, expirando
  e o bloqueio expirando.
- `configs`: o parse do `RATE_LIMIT_TOKENS` e a recusa de limites e tempo de
  bloqueio zerados ou negativos.

## Configuração

Tudo vem de variável de ambiente ou do `.env` na raiz. Variável de ambiente
ganha do `.env`, então dá pra sobrescrever qualquer coisa no `environment` do
compose sem mexer no arquivo.

| Variável | Padrão | O que faz |
|---|---|---|
| `WEB_SERVER_PORT` | `8080` | porta do servidor |
| `RATE_LIMIT_IP` | `10` | máximo de req/s por IP |
| `RATE_LIMIT_TOKEN` | `100` | máximo de req/s pra qualquer token que não esteja na lista abaixo |
| `RATE_LIMIT_TOKENS` | vazio | limites específicos por token, no formato `token:limite,token:limite` |
| `BLOCK_DURATION` | `300` | tempo de bloqueio em segundos depois de estourar o limite |
| `LIMITER_STORAGE` | `redis` | estratégia de persistência: `redis` ou `memory` |
| `REDIS_HOST` | `localhost` | no compose é sobrescrito pra `redis` |
| `REDIS_PORT` | `6379` | |
| `REDIS_PASSWORD` | vazio | |
| `REDIS_DB` | `0` | |

Os limites e o `BLOCK_DURATION` precisam ser maiores que zero. Com outro valor, a
aplicação não sobe e mostra o erro.

Exemplo:

```
RATE_LIMIT_IP=10
RATE_LIMIT_TOKEN=100
RATE_LIMIT_TOKENS=abc123:100,xyz789:50
BLOCK_DURATION=300
```

Aqui um IP sem token pode fazer 10 req/s, o `abc123` pode fazer 100, o `xyz789`
pode fazer 50 e qualquer outro token pode fazer 100.

### Token x IP

Se a requisição vem com o header `API_KEY`, quem manda é o token e o IP é
ignorado. A contagem e o bloqueio passam a ser do token. Então:

- um IP bloqueado continua conseguindo fazer requisição se mandar um token válido
- um token bloqueado fica bloqueado independente de qual IP esteja usando ele
- se o token tiver um limite menor que o do IP, vale o do token também

Não tem cadastro de tokens nem validação, qualquer valor no header é tratado
como token. Os que estão em `RATE_LIMIT_TOKENS` ganham o limite deles e o resto
cai no `RATE_LIMIT_TOKEN`.

### Sobre o IP

O IP vem do `RemoteAddr` da conexão. Não leio o `X-Forwarded-For` de propósito,
porque qualquer cliente pode mandar esse header com o valor que quiser e fugir do
limite. Se a aplicação for ficar atrás de um proxy ou load balancer, o caminho é
colocar o `middleware.RealIP` do chi antes do rate limiter no `main.go`.

## Como funciona

A ideia foi separar em três partes que não se conhecem mais do que o necessário:

```
cmd/server/main.go                         monta tudo: config, storage, limiter e rotas
configs/config.go                          leitura do .env com viper
internal/limiter/storage.go                interface Storage (a estratégia)
internal/limiter/limiter.go                regra de negócio
internal/infra/storage/redis.go            estratégia com Redis
internal/infra/storage/memory.go           estratégia em memória
internal/infra/web/middleware/ratelimit.go middleware HTTP
```

**Limiter** (`internal/limiter`): não sabe nada de HTTP nem de Redis. Recebe um IP
e um token, decide qual chave e qual limite usar, e pergunta pro storage se
estourou. O fluxo do `Allow` é:

1. se tem token, a chave é `token:<valor>` com o limite do token, senão é
   `ip:<valor>` com o limite do IP
2. se a chave está bloqueada, nega
3. incrementa o contador da chave na janela de 1 segundo
4. se passou do limite, bloqueia a chave pelo `BLOCK_DURATION` e nega

**Middleware** (`internal/infra/web/middleware`): só tira o IP e o header
`API_KEY` da requisição, chama o `Allow` e devolve 429 ou segue pro próximo
handler. É um `func(http.Handler) http.Handler` comum, então funciona com o chi
(que é o que estou usando) ou com qualquer outro router.

**Storage**: o contador é uma janela fixa de 1 segundo. No Redis é um `INCR`
numa chave que ganha TTL de 1s no primeiro incremento. O `INCR` e o `PEXPIRE`
rodam num script Lua pra ser atômico, porque se fossem dois comandos e a
aplicação caísse entre eles a chave ficaria sem TTL e nunca mais zerava. O
bloqueio é só um `SET` com TTL do tempo de bloqueio, e o Redis apaga sozinho
quando vence.

Se o Redis cair, o middleware loga o erro e deixa a requisição passar. Preferi a
API continuar no ar sem limite do que cair junto com o Redis. A consulta ao
storage tem um prazo de 500 ms, então enquanto o Redis estiver fora cada
requisição espera no máximo esse tempo antes de seguir.

## Trocando a estratégia de persistência

O limiter depende só desta interface:

```go
type Storage interface {
	Increment(ctx context.Context, key string, window time.Duration) (int64, error)
	Block(ctx context.Context, key string, duration time.Duration) error
	IsBlocked(ctx context.Context, key string) (bool, error)
}
```

Já tem duas implementações, Redis e memória. Pra usar a de memória é só trocar
no `.env`:

```
LIMITER_STORAGE=memory
```

A de memória é útil pra teste, mas não serve pra produção com mais de uma
instância, já que cada uma teria seus próprios contadores.

Pra colocar uma terceira (Memcached, Postgres, o que for):

1. criar um arquivo em `internal/infra/storage` com uma struct que implemente os
   três métodos da interface
2. adicionar um `case` no `newStorage` do `cmd/server/main.go` criando ela
3. usar o nome novo no `LIMITER_STORAGE`

O limiter e o middleware não mudam.
