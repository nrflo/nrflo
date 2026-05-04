import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from 'recharts'
import type { ThroughputPoint } from '@/types/insights'

interface Props {
  data: ThroughputPoint[]
}

export function ThroughputChart({ data }: Props) {
  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="time" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} />
        <Tooltip />
        <Area
          type="monotone"
          dataKey="success"
          stackId="1"
          stroke="#22c55e"
          fill="#22c55e"
          fillOpacity={0.4}
          name="Success"
        />
        <Area
          type="monotone"
          dataKey="error"
          stackId="1"
          stroke="#ef4444"
          fill="#ef4444"
          fillOpacity={0.4}
          name="Error"
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}
