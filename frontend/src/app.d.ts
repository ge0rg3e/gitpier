// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	interface ElectronWindowControls {
		minimize: () => Promise<void>;
		toggleMaximize: () => Promise<boolean>;
		close: () => Promise<void>;
		isMaximized: () => Promise<boolean>;
		onWindowStateChange: (listener: (state: { maximized: boolean }) => void) => () => void;
	}

	interface Window {
		electronWindowControls?: ElectronWindowControls;
		__gitpier_config?: {
			sshCloneHost?: string;
			turnstileSiteKey?: string;
		};
	}

	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
