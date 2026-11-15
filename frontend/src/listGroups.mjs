// Fork (integrations SUBSCRIBERS-FILTERS-2-SPEC D5). The ONE grouping of a Lists dropdown's options,
// shared by the Subscribers page (subscriberFilters.mjs::listOptions) and the Campaigns page
// (campaignsRouteQuery.mjs::campaignListOptions): the ordinary lists by name, then the system lists
// (the server's journey_locked, listTrigger.mjs::journeyLockedOf) by name, for an <optgroup>
// "System lists". Each page keeps its own rules for WHICH lists are offered and when the control is
// enabled. Pure; tested by listGroups.test.mjs.
import { journeyLockedOf } from './listTrigger.mjs'; // eslint-disable-line import/extensions

const nameOf = (l) => (l && typeof l.name === 'string' ? l.name : '');
const byName = (a, b) => a.name.localeCompare(b.name);

// groupLists -> { ordinary, system }: { id, name } rows, each group sorted by name.
// eslint-disable-next-line import/prefer-default-export
export function groupLists(lists) {
  const ordinary = [];
  const system = [];
  (Array.isArray(lists) ? lists : []).forEach((l) => {
    if (!l) {
      return;
    }
    (journeyLockedOf(l) ? system : ordinary).push({ id: l.id, name: nameOf(l) });
  });
  ordinary.sort(byName);
  system.sort(byName);
  return { ordinary, system };
}
