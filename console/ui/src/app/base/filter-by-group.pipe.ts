import { Pipe, PipeTransform } from '@angular/core';

interface Route {
  group: string;
  navItem: string;
  routerLink: string[];
  label: string;
  minRole: number;
  icon: string;
}

@Pipe({
  name: 'filterByGroup',
  pure: true
})
export class FilterByGroupPipe implements PipeTransform {
  transform(routes: Route[] | null, group: string): Route[] {
    if (!routes || !group) {
      return [];
    }
    return routes.filter((route: Route) => route.group === group);
  }
} 