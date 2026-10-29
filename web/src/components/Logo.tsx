/** The bearing mark: an outer ring, the balls and the inner race. */
export function Mark({ size = 24 }: { size?: number }) {
  return (
    <svg viewBox="0 0 64 64" width={size} height={size} aria-hidden="true">
      <circle cx="32" cy="32" r="29" fill="none" className="stroke-accent" strokeWidth="6" />
      <circle cx="32" cy="32" r="11" fill="none" className="stroke-ink" strokeWidth="6" />
      <g className="fill-ink">
        {[0, 45, 90, 135, 180, 225, 270, 315].map((angle) => (
          <circle key={angle} cx={32 + 19.5 * Math.sin((angle * Math.PI) / 180)} cy={32 - 19.5 * Math.cos((angle * Math.PI) / 180)} r="4.5" />
        ))}
      </g>
    </svg>
  );
}
