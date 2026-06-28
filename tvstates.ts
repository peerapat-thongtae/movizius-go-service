export interface TVStates {
  id: number
  user_id: string
  episode_watched: EpisodeWatched[]
  watchlisted_at: string
  max_watched_ep: MaxWatchedEp
  count_watched: number
  latest_watched: string
  account_status: string
  latest_state: string
  name: string
  media_type: string
  is_anime: boolean
  vote_average: number
  vote_count: number
  number_of_episodes: number
  number_of_seasons: number
  next_episode_to_air: any
  last_episode_to_air: LastEpisodeToAir
  seasons: Season[]
  watched_seasons: any[]
}

export interface EpisodeWatched {
  episode_id: number
  season_number: number
  episode_number: number
  watched_at: string
}

export interface MaxWatchedEp {
  episode_id: number
  season_number: number
  episode_number: number
  watched_at: string
}

export interface LastEpisodeToAir {
  id: number
  episode_number: number
  season_number: number
  air_date: string
  episode_type: string
}

export interface Season {
  air_date: string
  name: string
  episode_count: number
  season_number: number
  id: number
}