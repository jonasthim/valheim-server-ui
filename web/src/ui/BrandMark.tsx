/** The app mark: an ember-gradient rounded square with a stylised "V" horn. */
export function BrandMark({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden focusable="false">
      <defs>
        <linearGradient id="vhBrandGrad" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#ffc453" />
          <stop offset="1" stopColor="#c97c05" />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="30" height="30" rx="8" fill="url(#vhBrandGrad)" />
      <path d="M8 9l8 15 8-15" fill="none" stroke="#1a1206" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M11.5 9h9" stroke="#1a1206" strokeWidth="2.2" strokeLinecap="round" opacity="0.7" />
    </svg>
  )
}
