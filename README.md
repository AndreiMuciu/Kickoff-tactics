# Kickoff Tactics

[![CI](https://github.com/AndreiMuciu/Kickoff-tactics/actions/workflows/ci.yml/badge.svg)](https://github.com/AndreiMuciu/Kickoff-tactics/actions/workflows/ci.yml)

Auto-battler de fotbal. Ești antrenor: cumperi jucători, îi așezi pe teren, ei joacă singuri,
iar la pauză dai ordinul pentru repriza a doua. Sezon solo contra echipe conduse de server,
serie live cu un prieten și lobby-uri de 8 manageri (normal, ranked, între prieteni).

Designul și deciziile sunt în [DESIGN.md](DESIGN.md).

## Pornire

Cerințe: Go și Node (testat cu Go 1.27, Node 26). MongoDB este opțional.

```bash
./run.sh            # backend pe 127.0.0.1:8080, frontend pe http://localhost:5173
```

Sau separat:

```bash
cd backend && go run ./cmd/server
cd frontend && npm ci && npm run dev
```

Fără `MONGODB_URI`, starea stă în memorie și se pierde la repornire. Cu MongoDB:

```bash
docker run -d --name footy-mongo -p 127.0.0.1:27017:27017 mongo:8
cp .env.example .env    # setează MONGODB_URI=mongodb://127.0.0.1:27017
./run.sh
```

Pentru MongoDB Atlas pui în `.env` URI-ul `mongodb+srv://...` al clusterului. Codul e același;
calea Atlas nu a fost încă rulată pe un cluster real.

Fișierul `.env` stă în rădăcina proiectului, lângă `run.sh`, care îl încarcă la pornire. Este
ignorat de git. Toate variabilele sunt descrise în [.env.example](.env.example).

## Publicare

Frontendul este static și merge pe Cloudflare Pages; serverul Go rulează separat, pe orice
gazdă cu HTTPS.

**Cloudflare Pages** (Workers & Pages > Create > Pages > Connect to Git):

| Setare | Valoare |
| --- | --- |
| Root directory | `frontend` |
| Build command | `npm run build` |
| Build output directory | `dist` |
| Variabilă `BACKEND_URL` | adresa serverului Go, de exemplu `https://api.example.com` |
| Secret `PROXY_SECRET` | același șir ca în `.env`-ul de pe server |

Versiunea de Node vine din `frontend/.node-version`. Funcția
`frontend/functions/api/[[path]].js` trimite `/api/*` (inclusiv WebSocket-ul `/api/ws`) către
`BACKEND_URL`, deci browserul vorbește cu o singură origine, la fel ca prin proxy-ul Vite local.
Pentru test local cu `wrangler pages dev`, cele două valori se pun în `frontend/.dev.vars`.

**Serverul Go** rulează în Docker pe un server propriu, în spatele lui Caddy. Fișierele sunt în
[deploy/](deploy/). Caddy refuză orice cerere fără `PROXY_SECRET`, deci serverul Go e accesibil
doar prin site, iar adresa jucătorului vine de la funcția Pages.

Domeniul API-ului stă în spatele proxy-ului Cloudflare (nor portocaliu), cu SSL/TLS pe modul
**Full**. Certificatul public e al lui Cloudflare și se reînnoiește singur; între Cloudflare și
server, Caddy folosește un certificat propriu, pe care și-l reînnoiește tot singur.

Pe server, o singură dată:

```bash
# Docker cu pluginul compose instalat; porturile 80 și 443 deschise; DNS: api.example.com -> IP (proxied)
sudo mkdir -p /opt/kickoff && sudo chown deploy: /opt/kickoff     # deploy = userul de SSH, în grupul docker
nano /opt/kickoff/.env && chmod 600 /opt/kickoff/.env             # după modelul deploy/.env.example
```

**GitHub Actions**

| Workflow | Când | Ce face |
| --- | --- | --- |
| `ci.yml` | orice push și pull request | format, vet, teste Go; typecheck și build frontend |
| `build-and-push.yml` | push pe `main` care atinge `backend/` | teste, apoi imagine `linux/amd64` + `linux/arm64` pe Docker Hub (`<commit>` și `latest`) |
| `deploy.yml` | după `build-and-push.yml` reușit, sau manual cu un tag | copiază `deploy/` pe server, `docker compose pull && up -d`, așteaptă `/healthz` |

Setări în repo (Settings > Secrets and variables > Actions):

| Nume | Tip | Valoare |
| --- | --- | --- |
| `DOCKERHUB_USERNAME` | variabilă | utilizatorul Docker Hub |
| `DOCKERHUB_TOKEN` | secret | access token Docker Hub cu drept de scriere |
| `SSH_HOST` | secret, mediul `production` | adresa serverului |
| `SSH_USER` | secret, mediul `production` | userul de deploy |
| `SSH_PRIVATE_KEY` | secret, mediul `production` | cheie privată folosită doar pentru deploy |
| `SSH_KNOWN_HOSTS` | secret, mediul `production` | ieșirea comenzii `ssh-keyscan -t ed25519 <adresa>` |
| `SSH_PORT` | variabilă, opțional | portul SSH dacă nu e 22 |

O schimbare doar în `deploy/` se aplică rulând manual `deploy.yml`. Întoarcerea la o versiune
mai veche: `deploy.yml` manual, cu sha-ul acelui commit ca tag.

Stiva din `deploy/` a fost rulată local (poartă cu secret, `/healthz`, verificarea de origine).
Publicarea pe Cloudflare Pages și pe server nu a fost încă rulată cap-coadă.

## Structură

```
backend/
  cmd/server/          pornire, configurare din variabile de mediu
  internal/game/       regulile: date, economie, magazin, simularea meciului (fără HTTP)
  internal/store/      persistență: memorie sau MongoDB
  internal/api/        HTTP: sesiune, limitare, validare
frontend/
  src/api.ts           clientul HTTP și tipurile
  src/App.tsx          ecranul de management
  src/MatchView.tsx    redarea meciului pe canvas
```

## Cine decide ce

Serverul decide tot. Clientul trimite doar intenții (`buy`, `sell`, `move`, `reroll`, `levelup`,
start meci, ordinul de la pauză) și desenează ce primește înapoi: cadre de poziții și evenimente.
Bugetul, magazinul, zarurile și rezultatul nu există în browser decât ca afișaj.

## Securitate

| Risc | Măsură |
| --- | --- |
| Client modificat (buget, jucători, rezultat) | Toată starea și toate regulile sunt pe server; răspunsul către client e o proiecție (`RunView`) fără seed și fără starea generatorului |
| Ghicirea sau alegerea zarurilor | Seed din `crypto/rand`, ținut secret până la final |
| Server care „aranjează” meciul după ordinul tău | Commit-reveal: SHA-256 al seed-ului e afișat înainte de alegere, seed-ul după meci; repriza întâi e prefix exact al meciului complet (test) |
| Furt de sesiune prin XSS | Token de 256 de biți în cookie `HttpOnly`, `SameSite=Strict`, `Path=/api`; în bază se ține doar hash-ul |
| Parole | Argon2id (19 MiB, 2 treceri) cu sare aleatoare; maximum 4 hash-uri simultan; parola nu e logată și nu iese din server |
| Ghicirea parolelor | Limită per IP și per cont; răspuns și timp identice pentru utilizator inexistent și parolă greșită |
| Fixarea sesiunii | Token nou la fiecare autentificare; logout șterge sesiunea din bază |
| Acces la conversațiile altora | Chat și amicale doar între prieteni acceptați, verificat pe server la fiecare cerere |
| Conținut injectat în chat | Text simplu, maximum 500 de caractere, fără caractere de control; clientul îl afișează ca text |
| Privit înainte într-o serie live | Serverul trimite doar cadrele deja „întâmplate” plus 3 s; repriza a doua nu se calculează înainte de finalul pauzei; ordinul adversarului nu e trimis niciodată |
| CSRF | Verificare strictă de `Origin` pe orice cerere care schimbă stare, plus `SameSite=Strict` |
| Cheltuire dublă prin cereri paralele | Lacăt per sesiune și versiune optimistă la salvare (12 `reroll` simultane cu buget 3: unul singur trece) |
| Injecție în interogări Mongo | Filtre construite doar din valori de server cu tip fix; corpul JSON e decodat în structuri stricte |
| Corpuri mari, câmpuri ascunse | Limită 2 KB, `DisallowUnknownFields`, `Content-Type` obligatoriu, fără date după obiect |
| Abuz, forță brută | Limitare per IP: 15 cereri/s; sesiuni noi 10/minut |
| Conexiuni lente | Timeout-uri pe citire, scriere, antete; timeout de 5 s per cerere |
| Scurgeri de detalii interne | Erorile interne sunt doar în log; clientul primește mesaje fixe |
| Expunere accidentală | Ascultă pe `127.0.0.1`; secretele vin doar din mediu, `.env` e ignorat de git |

Antete pe răspunsurile API: `Content-Security-Policy: default-src 'none'`, `X-Frame-Options: DENY`,
`X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.

Paginile frontend-ului primesc pe Cloudflare Pages antetele din `frontend/public/_headers`.

De făcut înainte de orice expunere publică: HTTPS cu `COOKIE_SECURE=true`, CSP și pentru paginile
frontend-ului, un furnizor de email (SMTP) pentru confirmare și resetarea parolei.

## Teste

```bash
cd backend && go test ./...
```

Acoperă: determinism, repriza întâi ca prefix, echilibru (echipa mai bună câștigă 60-90%, goluri pe
meci în interval), simetria terenului, validarea intențiilor, unirea jucătorilor, commit-reveal.

## Poze și nume reale

Jucătorii au nume reale. Pozele se încarcă în browser din API-ul public Wikipedia, doar imagini cu
licență liberă (Wikimedia Commons), într-o singură cerere pe sesiune; fără poză liberă apar inițialele.
Browserul jucătorului contactează deci `en.wikipedia.org` și `*.wikimedia.org`.
Pentru o lansare publică e nevoie de licențe pentru nume și imagine; vezi DESIGN.md.

## Cunoscut

- `npm audit` raportează `picomatch` (ReDoS) adus de Vite. E unealtă de build, nu ajunge în browser.
- Fără `SMTP_HOST`, mailurile de confirmare și de resetare a parolei apar doar în logul serverului.
- Lipsesc itemele, autentificarea în doi pași și ștergerea contului. Vezi DESIGN.md.
- `dev-test-users.txt` (ignorat de git) ține conturile de test locale create la verificare.
