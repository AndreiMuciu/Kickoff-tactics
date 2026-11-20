// Streaks: five wins in a row set a manager's name on fire; five defeats in a
// row leave it reeling. Shown wherever a live match puts names side by side.
export const STREAK_MIN = 5

export function streakTitle(streak: number): string | undefined {
  if (streak >= STREAK_MIN) return `${streak} wins in a row: on fire!`
  if (streak <= -STREAK_MIN) return `${-streak} defeats in a row: the dressing room is lost`
  return undefined
}

export function StreakName({ name, streak = 0 }: { name: string; streak?: number }) {
  if (streak >= STREAK_MIN)
    return (
      <span className={`streak-name hot ${streak >= 8 ? 'blazing' : ''}`} title={streakTitle(streak)}>
        <span className="streak-icon" aria-hidden>
          🔥
        </span>
        <span className="streak-text">{name}</span>
        <span className="streak-badge">{streak}W</span>
      </span>
    )
  if (streak <= -STREAK_MIN)
    return (
      <span className={`streak-name cold ${streak <= -8 ? 'lost' : ''}`} title={streakTitle(streak)}>
        <span className="streak-icon" aria-hidden>
          💫
        </span>
        <span className="streak-text">{name}</span>
        <span className="streak-badge">{-streak}L</span>
      </span>
    )
  return <>{name}</>
}
