import { repos, API_BASE } from '$lib/api/client';

export interface MarkdownAssetField {
	username: string;
	repo: string;
	textarea: HTMLTextAreaElement;
	getValue: () => string;
	setValue: (next: string) => void;
	onUploadState?: (uploading: boolean) => void;
	onError?: (message: string) => void;
}

function mediaFiles(files: File[]): File[] {
	return files.filter((f) => f.type.startsWith('image/') || f.type.startsWith('video/'));
}

function toAbsoluteAssetUrl(assetUrl: string): string {
	if (assetUrl.startsWith('http://') || assetUrl.startsWith('https://')) return assetUrl;
	return `${API_BASE}${assetUrl}`;
}

async function uploadAndInsert(files: File[], field: MarkdownAssetField) {
	const items = mediaFiles(files);
	if (items.length === 0) return;

	field.onUploadState?.(true);
	field.onError?.('');
	try {
		const snippets: string[] = [];
		for (const file of items) {
			const uploaded = await repos.uploadMarkdownAsset(field.username, field.repo, file);
			const absoluteUrl = toAbsoluteAssetUrl(uploaded.asset_url);
			if (uploaded.content_type.startsWith('image/')) {
				snippets.push(`![${uploaded.original_name}](${absoluteUrl})`);
			} else {
				snippets.push(`<video controls preload="metadata" src="${absoluteUrl}"></video>`);
			}
		}

		const insertion = `${snippets.join('\n')}\n`;
		const start = field.textarea.selectionStart ?? field.getValue().length;
		const end = field.textarea.selectionEnd ?? start;
		const current = field.getValue();
		field.setValue(current.slice(0, start) + insertion + current.slice(end));
		setTimeout(() => {
			field.textarea.focus();
			const nextPos = start + insertion.length;
			field.textarea.setSelectionRange(nextPos, nextPos);
		}, 0);
	} catch (e: any) {
		const message = typeof e?.message === 'string' ? e.message : '';
		if (message.toLowerCase().includes('failed to fetch')) {
			field.onError?.('Upload failed: connection dropped. File may be too large, or backend may need restart.');
		} else {
			field.onError?.(message || 'Failed to upload pasted media');
		}
	} finally {
		field.onUploadState?.(false);
	}
}

export async function handleMarkdownPaste(event: ClipboardEvent, field: MarkdownAssetField) {
	const files: File[] = [];
	for (const item of Array.from(event.clipboardData?.items ?? [])) {
		if (item.kind === 'file') {
			const file = item.getAsFile();
			if (file) files.push(file);
		}
	}
	if (mediaFiles(files).length === 0) return;
	event.preventDefault();
	await uploadAndInsert(files, field);
}

export function handleMarkdownDragOver(event: DragEvent) {
	const files = Array.from(event.dataTransfer?.files ?? []);
	if (mediaFiles(files).length > 0) event.preventDefault();
}

export async function handleMarkdownDrop(event: DragEvent, field: MarkdownAssetField) {
	const files = Array.from(event.dataTransfer?.files ?? []);
	if (mediaFiles(files).length === 0) return;
	event.preventDefault();
	await uploadAndInsert(files, field);
}

export async function openMarkdownAssetPicker(field: MarkdownAssetField) {
	const input = document.createElement('input');
	input.type = 'file';
	input.multiple = true;
	input.accept = 'image/*,video/*';
	await new Promise<void>((resolve) => {
		input.onchange = async () => {
			const files = Array.from(input.files ?? []);
			if (files.length > 0) await uploadAndInsert(files, field);
			resolve();
		};
		input.click();
	});
}
