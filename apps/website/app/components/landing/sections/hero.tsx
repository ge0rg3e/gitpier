import { cn } from '@/lib/cn';
import { Section } from '@/components/landing/ui/section';
import { Button } from '@/components/ui/button';

interface HeroProps extends React.HTMLAttributes<HTMLElement> {}

export function Hero({ className, ...props }: HeroProps) {
  return (
    <Section id="hero" className={cn('relative fade-bottom overflow-hidden pb-0 sm:pb-0 md:pb-0 px-0!', className)} {...props}>
      <div className="absolute top-0 left-0 w-full h-[720px] bg-[url('/images/cctv-lines.png')] bg-cover bg-center bg-no-repeat">
        <div className="size-full bg-linear-to-t from-[#3a95ab]/50 to-background/10" />
      </div>

      <div className="max-w-container mx-auto flex flex-col">
        <div className="flex flex-col items-center gap-6 text-center sm:gap-12">
          <h1 className="relative z-10 inline-block text-5xl leading-[0.95] font-semibold text-balance drop-shadow-2xl sm:text-7xl md:text-8xl">
            Git collaboration for developers.
          </h1>
          <p className="text-md animate-appear text-muted-foreground relative z-10 max-w-[740px] font-medium text-balance opacity-0 delay-100 sm:text-xl">
            The open-source Git platform built around developers, repositories, pull requests, issues, CI/CD pipelines, and team collaboration, all in one place.
          </p>
          <div className="relative z-[999] flex flex-wrap items-center justify-center gap-3">
            <Button variant="default" size="lg" href="/docs/self-hosted/get-started">
              Self-host GitPier
            </Button>
            <Button variant="outline" size="lg" href="https://github.com/ge0rg3e/gitpier">
              Contribute to GitPier
            </Button>
          </div>
          <img
            src="/images/preview.png"
            alt="GitPier Preview"
            width={1248}
            height={765}
            className="w-full rounded-xl z-50 border border-white/20"
          />
        </div>
      </div>
    </Section>
  );
}
