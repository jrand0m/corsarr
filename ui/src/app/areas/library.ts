import Movies from '../screens/library/Movies.svelte';
import MovieDetail from '../screens/library/MovieDetail.svelte';
import SeriesList from '../screens/library/Series.svelte';
import SeriesDetail from '../screens/library/SeriesDetail.svelte';
import type { AreaModule } from '../areas';

export const screens = {
  movies: Movies, add: Movies, movie: MovieDetail,
  series: SeriesList, addSeries: SeriesList, show: SeriesDetail,
} as unknown as AreaModule['screens'];
