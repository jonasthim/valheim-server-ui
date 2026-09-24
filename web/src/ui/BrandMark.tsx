/** The app mark: a flat ember rounded square with a stylised "V" horn. */
export function BrandMark({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden focusable="false">
      <rect x="1" y="1" width="30" height="30" rx="8" fill="#F5A20F" />
      <path d="M8 9l8 15 8-15" fill="none" stroke="#0A0A0B" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M11.5 9h9" stroke="#0A0A0B" strokeWidth="2.2" strokeLinecap="round" opacity="0.7" />
    </svg>
  )
}
