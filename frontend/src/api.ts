// The client holds no game logic. It sends intents and renders what the
// server returns; the session lives in an HttpOnly cookie it cannot read.

export interface PlayerDef {
  id: string
  name: string
  cost: number
  style: string
  role: string
  stats: number[]
  ref?: number
  trait: string
  traitDesc: string
  active: boolean
  wiki?: string
}
export interface SynergyDef {
  id: string
  name: string
  kind: 'style' | 'role'
  tiers: number[]
  desc: string[]
}
export interface Tactic {
  id: string
  name: string
  desc: string
  keeper: string
}
export interface Meta {
  players: PlayerDef[]
  synergies: SynergyDef[]
  tactics: Tactic[]
  gkLine: number
  halfTicks: number
  benchSize: number
  rerollCost: number
  baseIncome: number
  interestStep: number
  interestMax: number
  xpCost: number
  storage: string
  augments?: AugmentDef[]
  augmentRounds?: number[]
  penaltyPrize?: number
  penaltyRounds?: number
}
export interface AugmentDef {
  id: string
  name: string
  desc: string
  kind: 'economy' | 'team' | 'crest'
}
export interface Unit {
  uid: string
  def: string
  stars: number
  line: number
  lane: number
  ban?: number // matches of suspension left
}
export interface SynergyView {
  id: string
  count: number
  active: number
}
export interface PlanView {
  players: string[] // player ids the manager is building towards
  synergies: SynergyView[] // what that team would have, all fielded
  maxOutfield: number
}
export interface HistoryEntry {
  round: number
  opponent: string
  gf: number
  ga: number
  pens: boolean
  won: boolean
  damage: number
  income: number
  seed: string
  commit: string
  note?: string // what came of a promise made before the match
  draw?: boolean // solo league: level at full time
}
export interface LeagueTeam {
  name: string
  you?: boolean
  p: number
  w: number
  d: number
  l: number
  gf: number
  ga: number
  pts: number
}
export interface LeagueResult {
  round: number
  home: string
  away: string
  hg: number
  ag: number
}
export interface LeagueView {
  table: LeagueTeam[]
  last: LeagueResult[] // the latest matchday's results
  played: number
  position: number
  points: number
  europe: number // last European place
  relegate: number // first relegation place
}
export interface StoryChoice {
  label: string
  effect: string
  cost?: number
}
export interface StoryEvent {
  id: string
  title: string
  text: string
  choices: StoryChoice[]
}
export interface RunView {
  phase: 'shop' | 'halftime' | 'ended'
  round: number
  maxRounds: number
  patience: number
  maxPatience: number
  budget: number
  level: number
  maxLevel: number
  xp: number
  xpNeed: number
  slots: number
  shop: string[]
  shopOdds: number[]
  units: Unit[]
  synergies: SynergyView[]
  opponent: { name: string; units: Unit[] }
  oppSynergies: SynergyView[]
  streak: number
  wins: number
  losses: number
  history: HistoryEntry[]
  result: string
  commit?: string
  locked?: boolean
  rerollCost?: number // what the next refresh costs (0 when it is free)
  interestMax?: number
  augments?: string[]
  augmentOffer?: string[]
  lossCost?: [number, number] // patience a defeat in the next match costs: by one goal, by three or more
  draws: number
  league?: LeagueView // solo: the table
  event?: StoryEvent // solo: a club event waiting for an answer
  pledge?: string // solo: a promise the next match will judge (win | big_win | clean)
}
export interface RosterEntry {
  idx: number
  team: number
  name: string
  def: string
  role: string
  stars: number
  gk: boolean
}
export interface GameEvent {
  t: number
  type: string
  team: number
  p: number
  q: number
  xg?: number
}
export interface TeamStats {
  shots: number
  xg: number
  passes: number
  tackles: number
  poss: number
}
// MatchData is what the replay needs; season halves and friendlies both carry it.
export interface MatchData {
  half: number
  roster: RosterEntry[]
  frames: number[][]
  events: GameEvent[] | null
  score: [number, number]
  stats: [TeamStats, TeamStats]
  pens?: { team: number; scored: boolean }[]
  names: [string, string]
}
export interface HalfData extends MatchData {
  result?: HistoryEntry
  run: RunView
  unlocked?: Achievement[] // achievements this match unlocked
}
export type Action =
  | { type: 'buy'; slot: number; def: string }
  | { type: 'sell'; uid: string }
  | { type: 'move'; uid: string; line: number; lane: number }
  | { type: 'reroll' }
  | { type: 'levelup' }
  | { type: 'lock' }
  | { type: 'augment'; slot: number }
  | { type: 'story'; slot: number }

export interface SeasonSummary {
  phase: 'shop' | 'halftime' | 'ended'
  round: number
  maxRounds: number
  wins: number
  draws: number
  losses: number
  patience: number
  result: string
  points: number // league points
  position: number // league position, 1-10
}
export interface Me {
  name: string
  hasEmail: boolean
  verified: boolean
  hasSquad: boolean
  season: SeasonSummary | null
  newAchievements: number
  askFeedback: boolean // a week has passed: ask whether there is anything to say about the game
}
export interface Achievement {
  id: string
  icon: string
  name: string
  desc: string
  target: number
}
export interface AchievementView extends Achievement {
  progress: number
  at?: number // unlock time (ms); absent while locked
  new?: boolean
}
export interface RankedSeason {
  number: number
  endsAt: number
}
export interface SeasonBadge {
  season: number
  tier: string
  points: number
}
export interface ProfileView {
  name: string
  email?: string // own profile only
  verified?: boolean
  friend?: boolean // a friend's profile, read-only
  since: number
  points: number
  tier: string
  rankedLobbies: number
  stats: Record<string, number>
  achievements: AchievementView[]
  season: RankedSeason
  seasonLobbies: number
  badges: SeasonBadge[]
}
export interface MatchLine {
  round: number
  opponent: string
  gf: number
  ga: number
  pens?: boolean
  won: boolean
  draw?: boolean
}
export interface GameRecord {
  id: string
  at: number
  mode: 'solo' | 'ranked' | 'normal' | 'friends' | 'series'
  season?: number
  result: string
  place?: number
  of?: number
  points?: number
  opponent?: string
  wins: number
  draws?: number
  losses: number
  matches: MatchLine[]
}
export interface HistoryPage {
  games: GameRecord[]
  page: number
  pages: number
  total: number
}
export interface Friend {
  name: string
  online: boolean
  hasSquad: boolean
}
export interface FriendList {
  friends: Friend[]
  incoming: string[]
  outgoing: string[]
}
export interface ChatMessage {
  id: string
  mine: boolean
  system: boolean
  text: string
  at: string
}

export interface SeriesResult {
  round: number
  score: [number, number]
  winner: number
  pens: boolean
}
export interface SeriesView {
  status: 'none' | 'invited' | 'live' | 'done'
  id?: string
  role: number
  opponent: string
  names: [string, string]
  round: number
  maxRounds: number
  phase?: 'shop' | 'countdown' | 'h1' | 'halftime' | 'h2' | 'shootout' | 'result' | 'done' | 'waiting'
  serverNow: number
  phaseEndsAt: number
  kickoffAt: number
  halfMs: number
  breakMs: number
  penTicks: number
  penRound?: boolean // the round is a penalty shootout and nothing else
  my?: RunView
  patience: [number, number]
  streaks?: [number, number] // +n wins or -n defeats in a row, per side
  maxPatience: number
  wins: [number, number]
  commit?: string
  roster?: RosterEntry[]
  from: number
  frames: number[][]
  events: GameEvent[]
  halfScore?: [number, number]
  stats?: [TeamStats, TeamStats]
  myOrder?: string
  score?: [number, number]
  pens?: { team: number; scored: boolean }[]
  matchWinner?: number
  seed?: string
  results: SeriesResult[]
  winner?: number
  // lobbies only
  ranked?: boolean
  standings?: Standing[]
  ghost?: boolean
  warmup?: boolean // this round is against a small club, nothing at stake
  warmupRounds?: number
  alive?: boolean
  place?: number
  lpChange?: number
  rating?: number
  tier?: string
  watching?: number // seat being spectated, -1 for your own match
  watched?: WatchedView
  spectators?: string[] // watching the match shown
  watchingMe?: string[] // watching your own match
}
export interface WatchedView {
  seat: number
  name: string
  level: number
  budget: number
  patience: number
  units: Unit[]
  synergies: SynergyView[]
  augments: string[]
}
export interface Standing {
  seat: number
  name: string
  bot: boolean
  you: boolean
  alive: boolean
  place: number
  patience: number
  streak?: number // +n wins or -n defeats in a row
  facing: boolean
  watchers?: number // managers spectating this one right now
}
export interface RankRow {
  rank: number
  name: string
  points: number
  tier: string
  games: number
  you: boolean
}
export interface Tier {
  name: string
  min: number
  max: number // -1: no upper limit
  points: number[] // change for places 1 to 8
}
export interface Ranking {
  players: RankRow[]
  total: number
  shown: number // at most this many rows are listed
  myRank: number
}
export interface RankedView {
  season: RankedSeason
  name: string
  rating: number
  tier: string
  games: number
  queued: boolean
  mode: '' | 'ranked' | 'normal'
  inQueue: number
  waitMs: number
  waitedMs: number
  spread: number // ranked search: points either side accepted now, -1 for anyone
  tiers: Tier[]
  lobbyId: string
  watchLobby?: string // a lobby still running that you are already out of
  watchPlace?: number
  party: { id: string; host: string; isHost: boolean; members: string[]; invited: string[] } | null
  invites: { id: string; host: string }[] | null
  top: { name: string; rating: number; tier: string; games: number }[] | null
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data?.error ?? 'Network error.')
  return data as T
}

export const api = {
  meta: () => call<Meta>('GET', '/api/meta'),

  signup: (username: string, email: string, password: string) => call<{ name: string }>('POST', '/api/auth/signup', { username, email, password }),
  verify: (token: string) => call<{ ok: boolean }>('POST', '/api/auth/verify', { token }),
  forgot: (email: string) => call<{ ok: boolean }>('POST', '/api/auth/forgot', { email }),
  reset: (token: string, password: string) => call<{ name: string }>('POST', '/api/auth/reset', { token, password }),
  profile: (name?: string) => call<ProfileView>('GET', name ? `/api/profile?name=${encodeURIComponent(name)}` : '/api/profile'),
  history: (page: number) => call<HistoryPage>('GET', `/api/history?page=${page}`),
  achievementsSeen: () => call<{ ok: boolean }>('POST', '/api/achievements/seen', {}),
  feedbackLater: () => call<{ ok: boolean }>('POST', '/api/feedback/later', {}),
  feedback: (kind: 'idea' | 'bug' | 'other', text: string, screen: string) =>
    call<{ ok: boolean; unlocked: Achievement[] }>('POST', '/api/feedback', { kind, text, screen }),
  changePassword: (current: string, next: string) => call<{ ok: boolean }>('POST', '/api/profile/password', { current, new: next }),
  changeEmail: (email: string, password: string) => call<{ ok: boolean }>('POST', '/api/profile/email', { email, password }),
  resendVerify: () => call<{ ok: boolean }>('POST', '/api/profile/resend', {}),
  login: (username: string, password: string) => call<{ name: string; restored?: boolean }>('POST', '/api/auth/login', { username, password }),
  deleteAccount: (password: string) => call<{ ok: boolean; purgeAt: number }>('POST', '/api/profile/delete', { password }),
  logout: () => call<{ ok: boolean }>('POST', '/api/auth/logout', {}),
  me: () => call<Me>('GET', '/api/me'),

  friends: () => call<FriendList>('GET', '/api/friends'),
  friendRequest: (username: string) => call<{ status: string }>('POST', '/api/friends/request', { username }),
  friendRespond: (username: string, accept: boolean) => call<{ ok: boolean }>('POST', '/api/friends/respond', { username, accept }),
  friendRemove: (username: string) => call<{ ok: boolean }>('POST', '/api/friends/remove', { username }),
  chatSend: (to: string, text: string) => call<ChatMessage>('POST', '/api/chat', { to, text }),

  seriesInvite: (username: string) => call<{ id: string }>('POST', '/api/series/invite', { username }),
  seriesRespond: (accept: boolean) => call<{ id?: string; ok?: boolean }>('POST', '/api/series/respond', { accept }),
  seriesCancel: () => call<{ ok: boolean }>('POST', '/api/series/cancel', {}),
  seriesLeave: (id: string) => call<{ ok: boolean }>('POST', '/api/series/leave', { id }),
  seriesAction: (id: string, a: Action) => call<RunView>('POST', '/api/series/action', { id, ...a }),
  seriesOrder: (id: string, tactic: string) => call<{ myOrder: string }>('POST', '/api/series/order', { id, tactic }),

  plan: () => call<PlanView>('GET', '/api/plan'),
  savePlan: (players: string[]) => call<PlanView>('POST', '/api/plan', { players }),
  ranked: () => call<RankedView>('GET', '/api/ranked'),
  ranking: (scope: 'all' | 'friends') => call<Ranking>('GET', `/api/ranking?scope=${scope}`),
  rankedJoin: (mode: 'ranked' | 'normal') => call<{ queued: boolean }>('POST', '/api/ranked/queue', { mode }),
  rankedCancel: () => call<{ queued: boolean }>('POST', '/api/ranked/cancel', {}),
  partyCreate: () => call<{ id: string }>('POST', '/api/party/create', {}),
  partyInvite: (username: string) => call<{ ok: boolean }>('POST', '/api/party/invite', { username }),
  partyJoin: (id: string, accept: boolean) => call<{ ok: boolean }>('POST', '/api/party/join', { id, accept }),
  partyLeave: () => call<{ ok: boolean }>('POST', '/api/party/leave', {}),
  partyStart: () => call<{ id: string }>('POST', '/api/party/start', {}),
  lobbyAction: (id: string, a: Action) => call<RunView>('POST', '/api/lobby/action', { id, ...a }),
  lobbyOrder: (id: string, tactic: string) => call<{ myOrder: string }>('POST', '/api/lobby/order', { id, tactic }),
  lobbyLeave: (id: string) => call<{ ok: boolean }>('POST', '/api/lobby/leave', { id }),

  run: () => call<RunView>('GET', '/api/run'),
  newRun: () => call<RunView>('POST', '/api/run'),
  action: (a: Action) => call<RunView>('POST', '/api/run/action', a),
  startMatch: () => call<HalfData>('POST', '/api/match/start'),
  match: () => call<HalfData>('GET', '/api/match'),
  secondHalf: (tactic: string) => call<HalfData>('POST', '/api/match/second-half', { tactic }),
}
