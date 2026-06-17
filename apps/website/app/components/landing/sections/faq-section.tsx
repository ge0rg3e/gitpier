import { Section } from '@/components/landing/ui/section';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';

interface FAQSectionProps extends React.HTMLAttributes<HTMLElement> {}

const selfHostURL = '/docs/self-hosted/get-started';

const items = [
  {
    value: 'is-open-source',
    question: 'Is this project open source?',
    title: 'Yes. GitPier is open source.',
    paragraphs: [
      'The project is developed in the open and the source code is publicly available.',
      'You can review the codebase, follow updates, and contribute improvements.',
    ],
    ctaLabel: 'View source code',
    ctaHref: 'https://github.com/ge0rg3e/gitpier',
  },
  {
    value: 'can-self-host',
    question: 'Can I self-host my own GitPier?',
    title: 'Yes, you can run your own GitPier instance.',
    paragraphs: [
      'You can deploy GitPier on your own infrastructure and manage your own repositories and organizations.',
      'Self-hosting gives you full control over data, access, and configuration.',
    ],
    ctaLabel: 'Self-hosting guide',
    ctaHref: selfHostURL,
  },
  {
    value: 'how-to-support',
    question: 'How can I support the project?',
    title: 'You can contribute directly to GitPier.',
    paragraphs: [
      'GitPier is developed in the open, and contributions help move the project forward.',
      'If you want to help, review the repository, open issues, or contribute code directly.',
    ],
    ctaLabel: 'Contribute to GitPier',
    ctaHref: 'https://github.com/ge0rg3e/gitpier',
  },
];

export function FAQSection({ className, ...props }: FAQSectionProps) {
  return (
    <Section className={className} {...props}>
      <div className="max-w-container mx-auto flex flex-col items-center gap-6 sm:gap-20">
        <h2 className="text-3xl leading-tight font-semibold sm:text-5xl sm:leading-tight text-center">
          Important notice
        </h2>
        <Accordion className="w-full max-w-[800px]" type="single" collapsible>
          {items.map((item) => (
            <AccordionItem key={item.value} value={item.value}>
              <AccordionTrigger>{item.question}</AccordionTrigger>
              <AccordionContent>
                <h2 className="text-lg font-semibold text-foreground">{item.title}</h2>
                {item.paragraphs.map((paragraph, i) => (
                  <p key={i} className="mt-2 text-sm text-muted-foreground">{paragraph}</p>
                ))}
                <a
                  href={item.ctaHref}
                  target="_blank"
                  rel="noreferrer"
                  className="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold text-primary hover:underline"
                >
                  {item.ctaLabel}
                </a>
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      </div>
    </Section>
  );
}
