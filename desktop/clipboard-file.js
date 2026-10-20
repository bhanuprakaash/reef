import { clipboard } from 'electron';
import { fileURLToPath } from 'node:url';

// A file copied in Finder is a text/uri-list entry; a text copy has none.
export async function clipboardFile() {
  const [item] = await clipboard.read();
  if (!item?.types.includes('text/uri-list')) return null;
  const list = await (await item.getType('text/uri-list')).text();
  const url = list.split(/\r?\n/)[0];
  if (!url.startsWith('file://')) return null;
  try {
    return fileURLToPath(url);
  } catch {
    return null;
  }
}
