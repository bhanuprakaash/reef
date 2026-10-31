import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { SkillPicker } from './skill-picker';

const hint = 'Pick a project to see its skills.';
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
        available={[
          { ref: 'claude:how', name: 'how', description: 'Explain.', source: 'claude' as const },
        ]}
        onChange={() => {}}
      />,
    );

    expect(markup).not.toContain(hint);
  });

  it('says a pick narrows the role to the picked skills', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={['claude:how']}
        available={[
          { ref: 'claude:how', name: 'how', description: 'Explain.', source: 'claude' as const },
        ]}
        limits
        onChange={() => {}}
      />,
    );

    expect(markup).toContain(onlyHint);
  });

  it('stays quiet about limits on a role whose backend does not enforce them', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={['claude:how']}
        available={[
          { ref: 'claude:how', name: 'how', description: 'Explain.', source: 'claude' as const },
        ]}
        onChange={() => {}}
      />,
    );

    expect(markup).not.toContain(onlyHint);
  });

  it('does not say it when nothing is picked', () => {
    const markup = renderToStaticMarkup(
      <SkillPicker
        label="dev"
        picked={[]}
        available={[
          { ref: 'claude:how', name: 'how', description: 'Explain.', source: 'claude' as const },
        ]}
        limits
        onChange={() => {}}
      />,
    );

    expect(markup).not.toContain(onlyHint);
  });
});
