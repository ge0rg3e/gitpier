import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ url, params }) => {
	const q = url.searchParams.toString();
	const suffix = q ? `?${q}` : '';
	throw redirect(307, `/apps/${params.slug}/install${suffix}`);
};
