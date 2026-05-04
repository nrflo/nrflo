import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Legend,
} from 'recharts'
import type { NrvappEditRateRow } from '@/types/nrvapp'

interface Props {
  data: NrvappEditRateRow[]
}

export function EditRateChart({ data }: Props) {
  return (
    <ResponsiveContainer width="100%" height={220}>
      <BarChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="tool_name" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} />
        <Tooltip />
        <Legend />
        <Bar dataKey="approve_no_edits" name="Approved (no edits)" stackId="a" fill="#22c55e" />
        <Bar
          dataKey="approve_with_edits"
          name="Approved (with edits)"
          stackId="a"
          fill="#84cc16"
        />
        <Bar dataKey="reject" name="Rejected" stackId="a" fill="#ef4444" />
      </BarChart>
    </ResponsiveContainer>
  )
}
