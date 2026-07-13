import { Badge } from '@/components/ui/Badge'
import type { PlanManifest, PlanQuestion } from '@/types/plan'

interface PlanManifestViewProps {
  manifest: PlanManifest
  questions?: PlanQuestion[]
}

// Read-only renderer for a plan manifest: layer -> policy -> nodes with
// template + instructions. Manifest editing is display-only here;
// PlanReviseDialog binds answers to these question ids separately.
export function PlanManifestView({ manifest, questions }: PlanManifestViewProps) {
  return (
    <div className="space-y-3">
      <p className="text-sm text-foreground">{manifest.goal}</p>
      <div className="space-y-2">
        {manifest.layers.map((layer) => (
          <div key={layer.layer} className="rounded-md border border-border p-2">
            <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground mb-1.5">
              <span>Layer {layer.layer}</span>
              <Badge variant="outline" className="text-xs">{layer.policy}</Badge>
            </div>
            <ul className="space-y-1.5">
              {layer.nodes.map((node) => (
                <li key={node.id} className="text-sm">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{node.id}</span>
                    <Badge variant="secondary" className="text-xs">{node.template}</Badge>
                  </div>
                  <p className="text-xs text-muted-foreground whitespace-pre-wrap">{node.instructions}</p>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      {questions && questions.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">Open questions</p>
          <ul className="space-y-1 list-disc list-inside">
            {questions.map((q) => (
              <li key={q.id} className="text-sm">
                <Badge variant="outline" className="text-xs mr-1.5">{q.id}</Badge>
                {q.question}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
