import * as React from 'react';
import { cn } from '@/lib/cn';

interface LayoutLinesProps extends React.HTMLAttributes<HTMLElement> {}

const LayoutLines = React.forwardRef<HTMLElement, LayoutLinesProps>(
  ({ className, ...props }, ref) => (
    <section ref={ref} className={cn('pointer-events-none fixed inset-0 top-0', className)} {...props}>
      <div className="max-w-container line-y line-dashed mx-auto flex h-full flex-col" />
    </section>
  )
);
LayoutLines.displayName = 'LayoutLines';

export { LayoutLines };
