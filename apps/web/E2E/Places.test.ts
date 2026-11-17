import { describe, expect, it } from 'vitest';

import { SECTIONS } from '../Src/App/Navigation/Sections';
import { isSlideId } from '../Src/App/Panels';
import { LOG_JOB_ID } from './LogFixture';
import { allPlaces, pagePlaces, SECTION_PLACES, SLIDE_EXEMPT, SLIDE_PAGES, slidePlaces, type IPlaceContext } from './Places';

const CONTEXT: IPlaceContext = { ProjectId: '01980000-0000-7000-8000-000000000001', Environment: 'production', RunId: '01980000-0000-7000-8000-000000000002' };

describe('the accessibility sweep covers every place', () => {
  it('has a place for every section with the same path', () => {
    const swept = new Set(pagePlaces(CONTEXT).map((place) => place.Path));
    for (const section of SECTIONS) {
      expect(SECTION_PLACES[section.id], section.id).toBe(section.path);
      expect(swept.has(section.path), section.id).toBe(true);
    }
    expect(Object.keys(SECTION_PLACES).sort()).toEqual(SECTIONS.map((section) => section.id).sort());
  });

  it('sweeps the deploy board and the log viewer with their fixtures', () => {
    const places = pagePlaces(CONTEXT);
    expect(places.find((place) => place.Path.endsWith('/deploy'))?.Stub).toBe('board');
    expect(places.find((place) => place.Path === `/jobs/${LOG_JOB_ID}`)?.Stub).toBe('log');
  });

  it('keeps the core pages first for the keyboard sweep', () => {
    expect(pagePlaces(CONTEXT).slice(0, 5).every((place) => place.Core === true)).toBe(true);
    expect(pagePlaces(CONTEXT).filter((place) => place.Core === true)).toHaveLength(5);
  });

  it('gives every slide a place or a written reason', () => {
    for (const [slide, builders] of Object.entries(SLIDE_PAGES)) {
      expect(isSlideId(slide), slide).toBe(true);
      if (builders.length === 0) {
        expect(SLIDE_EXEMPT[slide]?.length ?? 0, slide).toBeGreaterThan(0);
      }
    }
    for (const [slide, reason] of Object.entries(SLIDE_EXEMPT)) {
      expect(reason.length, slide).toBeGreaterThan(10);
    }
  });

  it('opens each slide through its own query', () => {
    const places = slidePlaces(CONTEXT);
    for (const [slide, builders] of Object.entries(SLIDE_PAGES)) {
      const found = places.filter((place) => place.Path.endsWith(`?slide=${slide}`));
      expect(found, slide).toHaveLength(builders.length);
    }
    expect(places.every((place) => place.Slide === true)).toBe(true);
  });

  it('has no duplicate name or path', () => {
    const places = allPlaces(CONTEXT);
    expect(new Set(places.map((place) => place.Name)).size).toBe(places.length);
    expect(new Set(places.map((place) => place.Path)).size).toBe(places.length);
  });
});
