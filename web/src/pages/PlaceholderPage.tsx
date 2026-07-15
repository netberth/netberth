import { PageLayout } from '@/components/layout/PageLayout'
import { Card, CardContent } from '@/components/ui/card'

interface Props {
  title: string
  icon?: string
}

export function PlaceholderPage({ title }: Props) {
  return (
    <PageLayout title={title} description="This module is under development">
      <Card className="border-border">
        <CardContent className="flex items-center justify-center py-24">
          <div className="text-center">
            <div className="mb-3 text-4xl font-bold text-muted-foreground/30">WIP</div>
            <p className="text-muted-foreground">The {title} module is coming soon.</p>
          </div>
        </CardContent>
      </Card>
    </PageLayout>
  )
}
