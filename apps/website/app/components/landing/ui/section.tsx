import * as React from 'react';
import { cn } from '@/lib/cn';

interface SectionProps extends React.HTMLAttributes<HTMLElement> {}

const Section = React.forwardRef<HTMLElement, SectionProps>(
  ({ className, children, ...props }, ref) => (
    <section
      ref={ref}
      data-slot="section"
      className={cn('line-b px-4 py-12 sm:py-24 md:py-32', className)}
      {...props}
    >
      {children}
    </section>
  )
);
Section.displayName = 'Section';

export { Section };
