interface StatCard {
  label: string
  value: string | number
  secondary?: string
}

interface Props {
  cards: StatCard[]
}

export function SummaryCards({ cards }: Props) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
      {cards.map((card, i) => (
        <div key={i} className="border border-border rounded-lg p-4 space-y-1 bg-background">
          <div className="text-xs text-muted-foreground">{card.label}</div>
          <div className="text-2xl font-semibold">{card.value}</div>
          {card.secondary && (
            <div className="text-xs text-muted-foreground">{card.secondary}</div>
          )}
        </div>
      ))}
    </div>
  )
}
