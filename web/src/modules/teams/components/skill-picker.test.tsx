import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { SkillPicker } from './skill-picker';

const hint = 'Add a project to see its skills.';

describe('SkillPicker', () => {
  it('says why nothing can be picked when there is no project', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker label="dev" picked={[]} available={undefined} noProject onChange={() => {}} />,
    );

    expect(markup).toContain(hint);
  });

  it('stays quiet when a project has skills', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={[]}
        available={[{ name: 'how', description: 'Explain.' }]}
        onChange={() => {}}
      />,
    );

    expect(markup).not.toContain(hint);
  });
});
