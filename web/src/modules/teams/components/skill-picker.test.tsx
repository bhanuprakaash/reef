import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { SkillPicker } from './skill-picker';

const hint = 'Add a project to see its skills.';
const onlyHint = 'Only these skills are available.';

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

  it('says a pick narrows the role to the picked skills', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={['how']}
        available={[{ name: 'how', description: 'Explain.' }]}
        onChange={() => {}}
      />,
    );

    expect(markup).toContain(onlyHint);
  });

  it('does not say it when nothing is picked', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={[]}
        available={[{ name: 'how', description: 'Explain.' }]}
        onChange={() => {}}
      />,
    );

    expect(markup).not.toContain(onlyHint);
  });
});
