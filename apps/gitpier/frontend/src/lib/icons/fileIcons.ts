type NodeType = 'blob' | 'tree';

const fileNameIconMap: Record<string, string> = {
	'dockerfile': 'docker',
	'makefile': 'settings',
	'tsconfig.json': 'tsconfig',
	'jsconfig.json': 'jsconfig',
	'readme.md': 'markdown',
	'license': 'license',
	'license.md': 'license',
	'license.txt': 'license',
	'.gitignore': 'git',
	'.gitattributes': 'git',
	'.editorconfig': 'editorconfig'
};

const extIconMap: Record<string, string> = {
	ts: 'typescript',
	tsx: 'reactjs',
	cts: 'typescript',
	mts: 'typescript',
	js: 'js',
	jsx: 'reactjs',
	mjs: 'js',
	cjs: 'js',
	json: 'json',
	jsonc: 'json',
	yml: 'yaml',
	yaml: 'yaml',
	md: 'markdown',
	markdown: 'markdown',
	go: 'go',
	py: 'python',
	rb: 'ruby',
	rs: 'rust',
	java: 'java',
	php: 'php',
	cs: 'csharp',
	c: 'c',
	cpp: 'cpp',
	cc: 'cpp',
	cxx: 'cpp',
	h: 'cppheader',
	hpp: 'cppheader',
	html: 'html',
	htm: 'html',
	css: 'css',
	scss: 'scss',
	sass: 'sass',
	less: 'less',
	xml: 'xml',
	toml: 'toml',
	ini: 'config',
	sh: 'shell',
	bash: 'shell',
	zsh: 'shell',
	fish: 'shell',
	svelte: 'svelte',
	vue: 'vue',
	sql: 'database',
	swift: 'swift',
	kt: 'kotlin',
	kts: 'kotlin',
	lua: 'lua',
	scala: 'scala',
	hs: 'haskell',
	dart: 'dart',
	lock: 'lock'
};

const folderNameIconMap: Record<string, string> = {
	src: 'src',
	docs: 'docs',
	doc: 'docs',
	test: 'test',
	tests: 'test',
	__tests__: 'test',
	spec: 'test',
	specs: 'test',
	public: 'public',
	assets: 'asset',
	images: 'images',
	img: 'images',
	scripts: 'script',
	config: 'config',
	'.github': 'github',
	'.gitlab': 'gitlab',
	node_modules: 'node',
	dist: 'dist',
	build: 'build',
	coverage: 'coverage'
};

function fileExtensionCandidates(fileName: string): string[] {
	const parts = fileName.toLowerCase().split('.');
	if (parts.length <= 1) return [];
	const out: string[] = [];
	for (let i = 1; i < parts.length; i++) {
		const ext = parts.slice(i).join('.');
		if (ext) out.push(ext);
	}
	return out;
}

function resolveFileIconName(fileName: string): string {
	const lower = fileName.toLowerCase();
	if (fileNameIconMap[lower]) return fileNameIconMap[lower];
	for (const ext of fileExtensionCandidates(lower)) {
		if (extIconMap[ext]) return extIconMap[ext];
	}
	if (extIconMap[lower]) return extIconMap[lower];
	return 'file';
}

function resolveFolderIconName(folderName: string): string {
	return folderNameIconMap[folderName.toLowerCase()] ?? 'folder';
}

function fileIconUrl(iconName: string): string {
	if (iconName === 'file') return '/images/file-icons/default_file.svg';
	return `/images/file-icons/file_type_${iconName}.svg`;
}

function folderIconUrl(iconName: string, opened: boolean): string {
	if (iconName === 'folder') {
		return opened ? '/images/file-icons/default_folder_opened.svg' : '/images/file-icons/default_folder.svg';
	}
	if (opened) return `/images/file-icons/folder_type_${iconName}_opened.svg`;
	return `/images/file-icons/folder_type_${iconName}.svg`;
}

export function resolveRepoTreeIconUrl(name: string, type: NodeType, options?: { opened?: boolean }): string {
	if (type === 'tree') return folderIconUrl(resolveFolderIconName(name), !!options?.opened);
	return fileIconUrl(resolveFileIconName(name));
}
