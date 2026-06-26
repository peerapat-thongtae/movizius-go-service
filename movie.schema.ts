private defaultPipeline: PipelineStage[] = [
    {
      $addFields: {
        // max_watched_ep: { $max: '$episode_watched' },
        max_watched_ep: {
          $reduce: {
            input: '$episode_watched',
            initialValue: null,
            in: {
              $cond: [
                {
                  $or: [
                    { $gt: ['$$this.season_number', '$$value.season_number'] },
                    {
                      $and: [
                        {
                          $eq: [
                            '$$this.season_number',
                            '$$value.season_number',
                          ],
                        },
                        {
                          $gt: [
                            '$$this.episode_number',
                            '$$value.episode_number',
                          ],
                        },
                      ],
                    },
                  ],
                },
                '$$this',
                '$$value',
              ],
            },
          },
        },
        count_watched: { $size: '$episode_watched' },
      },
    },
    {
      $addFields: {
        latest_watched: '$max_watched_ep.watched_at',
      },
    },
    {
      $lookup: {
        from: 'tv',
        localField: 'id',
        foreignField: 'id',
        as: 'tv',
      },
    },
    { $unwind: '$tv' },
    {
      $addFields: {
        account_status: {
          $cond: {
            if: {
              $and: [
                {
                  $eq: ['$count_watched', '$tv.number_of_episodes'],
                },
                {
                  $not: { $eq: ['$tv.status', 'Returning Series'] },
                },
              ],
            },
            then: 'watched',
            else: {
              $cond: {
                if: {
                  $and: [
                    { $gt: ['$count_watched', 0] },
                    // { $ne: ['$count_watched', '$tv.number_of_episodes'] },
                    {
                      $eq: [
                        '$max_watched_ep.season_number',
                        '$tv.last_episode_to_air.season_number',
                      ],
                    },
                    {
                      $eq: [
                        '$max_watched_ep.episode_number',
                        '$tv.last_episode_to_air.episode_number',
                      ],
                    },
                    {
                      $or: [
                        {
                          $eq: ['$tv.next_episode_to_air', null],
                        },
                        {
                          $gt: [
                            '$tv.next_episode_to_air.air_date',
                            dayjs().format('YYYY-MM-DD'),
                          ],
                        },
                      ],
                    },
                  ],
                },
                then: 'waiting_next_ep',
                else: {
                  $cond: {
                    if: { $gt: ['$count_watched', 0] },
                    then: 'watching',
                    else: 'watchlist',
                  },
                },
              },
            },
          },
        },
      },
    },
    {
      $addFields: {
        latest_state: {
          $cond: {
            if: { $eq: ['$account_status', 'watched'] },
            then: '$latest_watched',
            else: '$watchlisted_at',
          },
        },
      },
    },
    {
      $project: {
        _id: 0,
        id: 1,
        user_id: 1,
        name: '$tv.name',
        media_type: 'tv',
        is_anime: '$tv.is_anime',
        vote_average: '$tv.vote_average',
        vote_count: '$tv.vote_count',
        number_of_episodes: '$tv.number_of_episodes',
        number_of_seasons: '$tv.number_of_seasons',
        episode_watched: 1,
        latest_watched: 1,
        watchlisted_at: 1,
        count_watched: 1,
        account_status: 1,
        latest_state: 1,
        max_watched_ep: 1,
        next_episode_to_air: '$tv.next_episode_to_air',
        last_episode_to_air: '$tv.last_episode_to_air',
        // latest_watched_formatted: 1,
        // next_ep_formatted: 1,
        seasons: '$tv.seasons',
      },
    },
  ];